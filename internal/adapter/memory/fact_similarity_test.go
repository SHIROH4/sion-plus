package memory

import (
	"context"
	"testing"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

type fixedEmbedding struct{}

func (fixedEmbedding) Vectorize(_ context.Context, text string) ([]float32, error) {
	if text == "从事后端开发工作" || text == "职业方向是服务端工程" {
		return []float32{1, 0}, nil
	}
	return []float32{0, 1}, nil
}
func (fixedEmbedding) BatchVectorize(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i], _ = fixedEmbedding{}.Vectorize(ctx, text)
	}
	return out, nil
}
func (fixedEmbedding) Dimension() int    { return 2 }
func (fixedEmbedding) IsAvailable() bool { return true }

func TestSemanticContentSimilarity(t *testing.T) {
	tests := []struct {
		name  string
		a     string
		b     string
		merge bool
	}{
		{"spacing and filler", "正在开发一个名为 Aurora 的 Go 电商项目", "正在开发名为Aurora的Go电商项目", true},
		{"same architecture wording", "方案采用 Redis Lua 原子预扣和 MySQL 本地事务", "技术方案采用Redis Lua原子预扣、MySQL本地事务", true},
		{"opposite preference", "喜欢海盐茶", "不喜欢海盐茶", false},
		{"different preference target", "喜欢茉莉花茶", "喜欢咖啡", false},
		{"different number", "每天工作 8 小时", "每天工作 10 小时", false},
		{"related but distinct", "关注消息投递可靠性", "关注库存扣减原子性", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, got := semanticContentSimilarity(test.a, test.b)
			if got != test.merge {
				t.Fatalf("merge=%v, want %v", got, test.merge)
			}
		})
	}
}

func TestEmbeddingMatchHandlesLowLexicalOverlap(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	candidate := types.FactEntry{Entity: "master", RelationType: "identity", Content: "从事后端开发工作", Importance: 7}
	if err := store.SaveFact(ctx, &candidate); err != nil {
		t.Fatal(err)
	}
	worker := &MemoryWorker{store: store, embedding: fixedEmbedding{}, embeddingModelID: "fixed"}
	incoming := types.FactEntry{Entity: "master", RelationType: "identity", Content: "职业方向是服务端工程", Importance: 7}

	if _, lexical := semanticContentSimilarity(incoming.Content, candidate.Content); lexical {
		t.Fatal("test pair unexpectedly matched lexical path")
	}
	match := worker.findEquivalentFactByEmbedding(ctx, &incoming, []types.FactEntry{candidate})
	if match == nil || match.fact.ID != candidate.ID || match.reason != "embedding_semantic" {
		t.Fatalf("unexpected embedding match: %#v", match)
	}
	if len(incoming.Vector) != 2 {
		t.Fatal("incoming vector was not retained for persistence")
	}
}

func TestMemoryWorkerBackfillsExistingFactEmbeddings(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	fact := types.FactEntry{Entity: "master", RelationType: "identity", Content: "从事后端开发工作", Importance: 7}
	if err := store.SaveFact(ctx, &fact); err != nil {
		t.Fatal(err)
	}
	worker, cfg := newIncrementalTestWorker(t, store)
	worker.SetEmbeddingService(fixedEmbedding{}, "fixed")
	worker.Start(ctx, cfg)
	defer worker.Stop()

	waitForCondition(t, func() bool {
		stored, err := store.GetFact(ctx, fact.ID)
		return err == nil && len(stored.Vector) == 2 && stored.EmbeddingModelID == "fixed" && stored.EmbeddingTextSHA256 != ""
	})
}
