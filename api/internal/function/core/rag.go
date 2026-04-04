package service

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	cacheservice "ollama-gateway/internal/function/cache"
	contextservice "ollama-gateway/internal/function/context"
	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/internal/function/core/prompts"
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
	ollamaService   *OllamaService
	routerService   *RouterService
	qdrantService   *QdrantService
	memoryProvider  MemoryContextProvider
	contextResolver ContextResolver
	logger          *slog.Logger
	cache           cache.Cache
	repoRoots       []string
	responseCache   *ResponseCache
	distributed     *cacheservice.Service
	promptLang      string
	retrievalSem    chan struct{}
	poolObserver    interface {
		RegisterPool(name string, capacity int)
		ObservePoolAcquire(name string, waited bool)
		ObservePoolRelease(name string)
	}
}

type MemoryContextProvider interface {
	GetRelevantContextText(ctx context.Context, query string, topK int) (string, error)
}

type ContextResolver interface {
	ResolveContextFiles(input contextservice.ResolveInput) ([]contextservice.ResolvedFile, error)
}

var _ domain.RAGEngine = (*RAGService)(nil)

func NewRAGService(
	ollamaService *OllamaService,
	routerService *RouterService,
	qdrantService *QdrantService,
	logger *slog.Logger,
	cacheBackend cache.Cache,
	repoRoots []string,
	promptLang string,
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
		promptLang:    strings.ToLower(strings.TrimSpace(promptLang)),
		retrievalSem:  make(chan struct{}, 8),
	}
}

func (s *RAGService) SetRetrievalPool(size int, observer interface {
	RegisterPool(name string, capacity int)
	ObservePoolAcquire(name string, waited bool)
	ObservePoolRelease(name string)
}) {
	if s == nil {
		return
	}
	if size <= 0 {
		size = 8
	}
	s.retrievalSem = make(chan struct{}, size)
	s.poolObserver = observer
	if observer != nil {
		observer.RegisterPool("retrieval", size)
	}
}

func (s *RAGService) InvalidateResponseCache() {
	if s == nil || s.responseCache == nil {
		return
	}
	s.responseCache.Clear()
}

func (s *RAGService) SetMemoryProvider(provider MemoryContextProvider) {
	if s == nil {
		return
	}
	s.memoryProvider = provider
}

func (s *RAGService) SetContextResolver(resolver ContextResolver) {
	if s == nil {
		return
	}
	s.contextResolver = resolver
}

func (s *RAGService) SetDistributedCache(distributed *cacheservice.Service) {
	if s == nil {
		return
	}
	s.distributed = distributed
}

func (s *RAGService) search(query string) string {
	waited := s.acquireRetrievalSlot()
	defer s.releaseRetrievalSlot()
	if s.poolObserver != nil {
		s.poolObserver.ObservePoolAcquire("retrieval", waited)
		defer s.poolObserver.ObservePoolRelease("retrieval")
	}

	embedding, err := s.ollamaService.GetEmbedding("nomic-embed-text", query)
	if err != nil || len(embedding) == 0 {
		return ""
	}
	if s.qdrantService == nil {
		return ""
	}

	allowedPaths := s.resolveAllowedPaths(query)
	merged := make([]map[string]interface{}, 0)
	filtered := make([]map[string]interface{}, 0)
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
				if len(allowedPaths) == 0 || containsResolvedPath(allowedPaths, payloadPathOf(m)) {
					filtered = append(filtered, m)
				}
			}
		}
	}

	if len(merged) == 0 {
		return ""
	}
	if len(filtered) == 0 {
		filtered = merged
	}

	sort.Slice(filtered, func(i, j int) bool {
		return scoreOf(filtered[i]) > scoreOf(filtered[j])
	})
	if len(filtered) > 4 {
		filtered = filtered[:4]
	}

	return s.extractCode(map[string]interface{}{"result": toInterfaceSlice(filtered)})
}

func (s *RAGService) acquireRetrievalSlot() bool {
	if s == nil || s.retrievalSem == nil {
		return false
	}
	select {
	case s.retrievalSem <- struct{}{}:
		return false
	default:
		s.retrievalSem <- struct{}{}
		return true
	}
}

func (s *RAGService) releaseRetrievalSlot() {
	if s == nil || s.retrievalSem == nil {
		return
	}
	select {
	case <-s.retrievalSem:
	default:
	}
}

func (s *RAGService) resolveAllowedPaths(prompt string) map[string]struct{} {
	if s == nil || s.contextResolver == nil || strings.TrimSpace(prompt) == "" {
		return nil
	}
	resolved, err := s.contextResolver.ResolveContextFiles(contextservice.ResolveInput{
		Prompt:   prompt,
		TopK:     12,
		MaxDepth: 2,
	})
	if err != nil || len(resolved) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(resolved))
	for _, item := range resolved {
		p := strings.TrimSpace(item.Path)
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			allowed[filepath.Clean(abs)] = struct{}{}
			continue
		}
		allowed[filepath.Clean(p)] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func payloadPathOf(item map[string]interface{}) string {
	payload, _ := item["payload"].(map[string]interface{})
	path, _ := payload["path"].(string)
	return strings.TrimSpace(path)
}

func containsResolvedPath(allowed map[string]struct{}, candidate string) bool {
	if len(allowed) == 0 {
		return true
	}
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	if abs, err := filepath.Abs(candidate); err == nil {
		candidate = filepath.Clean(abs)
	} else {
		candidate = filepath.Clean(candidate)
	}
	_, ok := allowed[candidate]
	return ok
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
	cleanPrompt, selectedLang := extractPromptLangDirective(prompt)
	if selectedLang == "" {
		selectedLang = s.promptLang
	}
	if selectedLang == "" {
		selectedLang = "en"
	}

	repoScope := s.repoScope()
	semanticContext := ""
	if s.distributed != nil {
		if cachedRetrieval, ok, err := s.distributed.GetRetrieval(context.Background(), repoScope, cleanPrompt); err == nil && ok {
			semanticContext = cachedRetrieval
		}
	}
	if semanticContext == "" {
		semanticContext = s.search(cleanPrompt)
		if s.distributed != nil {
			_ = s.distributed.SetRetrieval(context.Background(), repoScope, cleanPrompt, semanticContext)
		}
	}

	memoryContext := ""
	if s.memoryProvider != nil {
		if ctx, err := s.memoryProvider.GetRelevantContextText(context.Background(), cleanPrompt, 3); err == nil {
			memoryContext = strings.TrimSpace(ctx)
		}
	}

	contextMaterial := semanticContext + "\n" + memoryContext + "\n" + selectedLang
	cacheKey := responseCacheKey(cleanPrompt + "|" + contextMaterial)
	if s.responseCache != nil {
		if cached, ok := s.responseCache.Get(cacheKey); ok {
			return cached, nil
		}
	}
	if s.distributed != nil {
		if entry, ok, err := s.distributed.Get(context.Background(), repoScope, cleanPrompt, contextMaterial); err == nil && ok {
			if strings.TrimSpace(entry.Response) != "" {
				if s.responseCache != nil {
					s.responseCache.Set(cacheKey, entry.Response)
				}
				return entry.Response, nil
			}
		}
	}

	fullPrompt := s.buildPromptWithContexts(cleanPrompt, selectedLang, semanticContext, memoryContext)
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
	if s.distributed != nil {
		_ = s.distributed.Set(context.Background(), repoScope, cleanPrompt, contextMaterial, cacheservice.Entry{
			Retrieval: semanticContext,
			Response:  out,
		})
	}
	return out, nil
}

func (s *RAGService) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	cleanPrompt, selectedLang := extractPromptLangDirective(prompt)
	if selectedLang == "" {
		selectedLang = s.promptLang
	}
	if selectedLang == "" {
		selectedLang = "en"
	}

	repoScope := s.repoScope()
	semanticContext := ""
	if s.distributed != nil {
		if cachedRetrieval, ok, err := s.distributed.GetRetrieval(context.Background(), repoScope, cleanPrompt); err == nil && ok {
			semanticContext = cachedRetrieval
		}
	}
	if semanticContext == "" {
		semanticContext = s.search(cleanPrompt)
		if s.distributed != nil {
			_ = s.distributed.SetRetrieval(context.Background(), repoScope, cleanPrompt, semanticContext)
		}
	}

	memoryContext := ""
	if s.memoryProvider != nil {
		if ctx, err := s.memoryProvider.GetRelevantContextText(context.Background(), cleanPrompt, 3); err == nil {
			memoryContext = strings.TrimSpace(ctx)
		}
	}
	contextMaterial := semanticContext + "\n" + memoryContext + "\n" + selectedLang
	cacheKey := responseCacheKey(cleanPrompt + "|" + contextMaterial)
	if s.responseCache != nil {
		if cached, ok := s.responseCache.Get(cacheKey); ok {
			return streamCachedResponse(cached, onChunk)
		}
	}
	if s.distributed != nil {
		if entry, ok, err := s.distributed.Get(context.Background(), repoScope, cleanPrompt, contextMaterial); err == nil && ok {
			if strings.TrimSpace(entry.Response) != "" {
				if s.responseCache != nil {
					s.responseCache.Set(cacheKey, entry.Response)
				}
				return streamCachedResponse(entry.Response, onChunk)
			}
		}
	}

	fullPrompt := s.buildPromptWithContexts(cleanPrompt, selectedLang, semanticContext, memoryContext)
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
	if s.distributed != nil {
		_ = s.distributed.Set(context.Background(), repoScope, cleanPrompt, contextMaterial, cacheservice.Entry{
			Retrieval: semanticContext,
			Response:  builder.String(),
		})
	}
	return nil
}

func (s *RAGService) buildPrompt(prompt string) string {
	cleanPrompt, selectedLang := extractPromptLangDirective(prompt)
	if selectedLang == "" {
		selectedLang = s.promptLang
	}
	if selectedLang == "" {
		selectedLang = "en"
	}
	semanticContext := s.search(cleanPrompt)
	memoryContext := ""
	if s.memoryProvider != nil {
		if ctx, err := s.memoryProvider.GetRelevantContextText(context.Background(), cleanPrompt, 3); err == nil {
			memoryContext = strings.TrimSpace(ctx)
		}
	}
	return s.buildPromptWithContexts(cleanPrompt, selectedLang, semanticContext, memoryContext)
}

func (s *RAGService) buildPromptWithContexts(cleanPrompt, selectedLang, semanticContext, memoryContext string) string {
	systemPrompt := prompts.Get(selectedLang, "rag_system")
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = "You are an expert Go assistant."
	}

	if semanticContext != "" {
		if memoryContext != "" {
			return systemPrompt + " Use this context: " + semanticContext + "\n\nHistorical memory: " + memoryContext + "\n\nQuestion: " + cleanPrompt
		}
		return systemPrompt + " Use this context: " + semanticContext + "\n\nQuestion: " + cleanPrompt
	}
	if memoryContext != "" {
		return systemPrompt + "\n\nHistorical memory: " + memoryContext + "\n\nQuestion: " + cleanPrompt
	}
	return systemPrompt + "\n\nQuestion: " + cleanPrompt
}

func (s *RAGService) repoScope() string {
	if s == nil || len(s.repoRoots) == 0 {
		return "global"
	}
	roots := append([]string(nil), s.repoRoots...)
	sort.Strings(roots)
	return strings.Join(roots, "|")
}

func extractPromptLangDirective(prompt string) (string, string) {
	lines := strings.Split(prompt, "\n")
	lang := ""
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[prompt_lang=") && strings.HasSuffix(trimmed, "]") {
			v := strings.TrimSuffix(strings.TrimPrefix(trimmed, "[prompt_lang="), "]")
			lang = strings.ToLower(strings.TrimSpace(v))
			continue
		}
		clean = append(clean, line)
	}
	return strings.Join(clean, "\n"), lang
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
