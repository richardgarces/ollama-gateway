package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmbeddingPoolContentionBasic(t *testing.T) {
	svc := &OllamaService{embeddingSem: make(chan struct{}, 2)}

	var waitedCount int64
	var wg sync.WaitGroup
	workers := 20

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if svc.acquireEmbeddingSlot() {
				atomic.AddInt64(&waitedCount, 1)
			}
			time.Sleep(2 * time.Millisecond)
			svc.releaseEmbeddingSlot()
		}()
	}
	wg.Wait()

	if waitedCount == 0 {
		t.Fatalf("expected contention in embedding pool, got waited_count=0")
	}
}

func TestRetrievalPoolContentionBasic(t *testing.T) {
	rag := &RAGService{retrievalSem: make(chan struct{}, 2)}

	var waitedCount int64
	var wg sync.WaitGroup
	workers := 20

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if rag.acquireRetrievalSlot() {
				atomic.AddInt64(&waitedCount, 1)
			}
			time.Sleep(2 * time.Millisecond)
			rag.releaseRetrievalSlot()
		}()
	}
	wg.Wait()

	if waitedCount == 0 {
		t.Fatalf("expected contention in retrieval pool, got waited_count=0")
	}
}
