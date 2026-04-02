package services

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/cache"
	"ollama-gateway/pkg/reposcope"
)

const (
	defaultRAGCacheTTLSeconds = 1800
	defaultRAGCacheMaxEntries = 500
	cachedStreamChunkSize     = 96
)

type responseCacheEntry struct {
	key       string
	value     string
	expiresAt time.Time
}

type ResponseCache struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	ll         *list.List
	items      map[string]*list.Element
}

func NewResponseCache(ttl time.Duration, maxEntries int) *ResponseCache {
	if ttl <= 0 {
		ttl = time.Duration(defaultRAGCacheTTLSeconds) * time.Second
	}
	if maxEntries <= 0 {
		maxEntries = defaultRAGCacheMaxEntries
	}
	return &ResponseCache{
		ttl:        ttl,
		maxEntries: maxEntries,
		ll:         list.New(),
		items:      make(map[string]*list.Element),
	}
}

func (c *ResponseCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	entry, ok := el.Value.(*responseCacheEntry)
	if !ok {
		c.ll.Remove(el)
		delete(c.items, key)
		return "", false
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		c.ll.Remove(el)
		delete(c.items, key)
		return "", false
	}
	c.ll.MoveToFront(el)
	return entry.value, true
}

func (c *ResponseCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		entry, _ := el.Value.(*responseCacheEntry)
		if entry == nil {
			entry = &responseCacheEntry{key: key}
			el.Value = entry
		}
		entry.value = value
		entry.expiresAt = time.Now().Add(c.ttl)
		c.ll.MoveToFront(el)
		return
	}

	entry := &responseCacheEntry{key: key, value: value, expiresAt: time.Now().Add(c.ttl)}
	el := c.ll.PushFront(entry)
	c.items[key] = el

	for c.ll.Len() > c.maxEntries {
		tail := c.ll.Back()
		if tail == nil {
			break
		}
		c.ll.Remove(tail)
		if tailEntry, ok := tail.Value.(*responseCacheEntry); ok {
			delete(c.items, tailEntry.key)
		}
	}
}

func (c *ResponseCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element)
}

type RAGService struct {
	ollamaService *OllamaService
	routerService *RouterService
	qdrantService *QdrantService
	logger        *slog.Logger
	cache         cache.Cache
	repoRoots     []string
	responseCache *ResponseCache
}

var _ domain.RAGEngine = (*RAGService)(nil)

func NewRAGService(
	ollamaService *OllamaService,
	routerService *RouterService,
	qdrantService *QdrantService,
	logger *slog.Logger,
	cacheBackend cache.Cache,
	repoRoots []string,
	ragCacheTTLSeconds int,
	ragCacheMaxEntries int,
) *RAGService {
	if logger == nil {
		logger = slog.Default()
	}
	ttlSeconds := ragCacheTTLSeconds
	if ttlSeconds <= 0 {
		ttlSeconds = defaultRAGCacheTTLSeconds
	}
	maxEntries := ragCacheMaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultRAGCacheMaxEntries
	}
	return &RAGService{
		ollamaService: ollamaService,
		routerService: routerService,
		qdrantService: qdrantService,
		logger:        logger,
		cache:         cacheBackend,
		repoRoots:     reposcope.CanonicalizeRoots(repoRoots),
		responseCache: NewResponseCache(time.Duration(ttlSeconds)*time.Second, maxEntries),
	}
}

func (s *RAGService) InvalidateResponseCache() {
	if s == nil || s.responseCache == nil {
		return
	}
	s.responseCache.Clear()
}

func (s *RAGService) search(query string) string {
	embedding, err := s.ollamaService.GetEmbedding("nomic-embed-text", query)
	if err != nil || len(embedding) == 0 {
		return ""
	}
	if s.qdrantService == nil {
		return ""
	}

	merged := make([]map[string]interface{}, 0)
	for _, collection := range s.collections() {
		result, err := s.qdrantService.Search(collection, embedding, 3)
		if err != nil {
			continue
		}
		items, ok := result["result"].([]interface{})
		if !ok {
			continue
		}
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				merged = append(merged, m)
			}
		}
	}

	if len(merged) == 0 {
		return ""
	}

	sort.Slice(merged, func(i, j int) bool {
		return scoreOf(merged[i]) > scoreOf(merged[j])
	})
	if len(merged) > 4 {
		merged = merged[:4]
	}

	return s.extractCode(map[string]interface{}{"result": toInterfaceSlice(merged)})
}

func (s *RAGService) extractCode(result map[string]interface{}) string {
	res, ok := result["result"].([]interface{})
	if !ok {
		return ""
	}

	context := ""
	for _, h := range res {
		item, _ := h.(map[string]interface{})
		payload, _ := item["payload"].(map[string]interface{})
		code, _ := payload["code"].(string)
		context += "\n---\n" + code
	}
	return context
}

func (s *RAGService) collections() []string {
	if len(s.repoRoots) == 0 {
		return []string{"repo_docs"}
	}
	collections := make([]string, 0, len(s.repoRoots))
	seen := make(map[string]struct{})
	for _, root := range s.repoRoots {
		name := reposcope.CollectionName(root)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		collections = append(collections, name)
	}
	return collections
}

func scoreOf(item map[string]interface{}) float64 {
	v, ok := item["score"]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		if strings.TrimSpace(t) == "" {
			return 0
		}
	}
	return 0
}

func toInterfaceSlice(items []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

func (s *RAGService) GenerateWithContext(prompt string) (string, error) {
	cacheKey := responseCacheKey(prompt)
	if s.responseCache != nil {
		if cached, ok := s.responseCache.Get(cacheKey); ok {
			return cached, nil
		}
	}

	fullPrompt := s.buildPrompt(prompt)
	var out string
	var err error
	if s.routerService == nil {
		out, err = s.ollamaService.Generate("gemma:2b", fullPrompt)
	} else {
		out, err = s.routerService.GenerateWithFallback(prompt, fullPrompt)
	}
	if err != nil {
		return "", err
	}
	if s.responseCache != nil {
		s.responseCache.Set(cacheKey, out)
	}
	return out, nil
}

func (s *RAGService) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	cacheKey := responseCacheKey(prompt)
	if s.responseCache != nil {
		if cached, ok := s.responseCache.Get(cacheKey); ok {
			return streamCachedResponse(cached, onChunk)
		}
	}

	fullPrompt := s.buildPrompt(prompt)
	var builder strings.Builder
	writeChunk := func(chunk string) error {
		builder.WriteString(chunk)
		return onChunk(chunk)
	}
	var err error
	if s.routerService == nil {
		err = s.ollamaService.StreamGenerate("gemma:2b", fullPrompt, writeChunk)
	} else {
		err = s.routerService.StreamGenerateWithFallback(prompt, fullPrompt, writeChunk)
	}
	if err != nil {
		return err
	}
	if s.responseCache != nil {
		s.responseCache.Set(cacheKey, builder.String())
	}
	return nil
}

func (s *RAGService) buildPrompt(prompt string) string {
	context := s.search(prompt)
	if context != "" {
		return "Eres un experto en Go. Usa este contexto: " + context + "\n\nPregunta: " + prompt
	}
	return "Eres un experto en Go.\n\nPregunta: " + prompt
}

func responseCacheKey(prompt string) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(prompt))), " ")
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

func streamCachedResponse(cached string, onChunk func(string) error) error {
	if cached == "" {
		return nil
	}
	runes := []rune(cached)
	for i := 0; i < len(runes); i += cachedStreamChunkSize {
		end := i + cachedStreamChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		if err := onChunk(string(runes[i:end])); err != nil {
			return err
		}
	}
	return nil
}
