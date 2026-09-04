package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

const semanticFactMergeThreshold = 0.82
const embeddingFactMergeThreshold = 0.93

var numberPattern = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

type factMatch struct {
	fact       types.FactEntry
	reason     string
	similarity float64
}

// findEquivalentFact deliberately favors false negatives over false positives.
// Facts are eligible only within the same entity/relation bucket, and polarity,
// numeric values and large length differences act as hard conflict guards.
func findEquivalentFact(incoming types.FactEntry, candidates []types.FactEntry) *factMatch {
	incomingKey := normalizedFactKey(incoming)
	best := factMatch{}
	for _, candidate := range candidates {
		if candidate.Archived || !sameFactBucket(incoming, candidate) {
			continue
		}
		if normalizedFactKey(candidate) == incomingKey {
			return &factMatch{fact: candidate, reason: "normalized_exact", similarity: 1}
		}
		score, ok := semanticContentSimilarity(incoming.Content, candidate.Content)
		if ok && score > best.similarity {
			best = factMatch{fact: candidate, reason: "lexical_semantic", similarity: score}
		}
	}
	if best.fact.ID == 0 {
		return nil
	}
	return &best
}

func sameFactBucket(a, b types.FactEntry) bool {
	return normalizeFactField(a.Entity) == normalizeFactField(b.Entity) &&
		normalizeFactField(a.RelationType) == normalizeFactField(b.RelationType)
}

func normalizeFactField(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func semanticContentSimilarity(a, b string) (float64, bool) {
	na, nb := compactSemanticText(a), compactSemanticText(b)
	if na == "" || nb == "" || na == nb {
		if na != "" && na == nb {
			return 1, true
		}
		return 0, false
	}
	if hasNegativePolarity(a) != hasNegativePolarity(b) {
		return 0, false
	}
	if !sameNumbers(a, b) {
		return 0, false
	}
	lenA, lenB := len([]rune(na)), len([]rune(nb))
	lengthRatio := float64(minInt(lenA, lenB)) / float64(maxInt(lenA, lenB))
	if lengthRatio < 0.58 {
		return 0, false
	}

	gramsA, gramsB := runeNGrams(na, 2), runeNGrams(nb, 2)
	intersection := setIntersectionSize(gramsA, gramsB)
	dice := 2 * float64(intersection) / float64(len(gramsA)+len(gramsB))
	containment := float64(intersection) / float64(minInt(len(gramsA), len(gramsB)))
	score := 0.75*dice + 0.25*containment
	eligible := score >= semanticFactMergeThreshold || (containment >= 0.94 && lengthRatio >= 0.68)
	return math.Min(score, 1), eligible
}

func (w *MemoryWorker) findEquivalentFactByEmbedding(ctx context.Context, incoming *types.FactEntry, candidates []types.FactEntry) *factMatch {
	if w.embedding == nil {
		return nil
	}
	incomingVector, err := w.embedding.Vectorize(ctx, incoming.Content)
	if err != nil || len(incomingVector) == 0 {
		return nil
	}
	incoming.Vector = incomingVector
	incoming.EmbeddingModelID = w.embeddingModelID
	incoming.EmbeddingTextSHA256 = factContentSHA256(incoming.Content)
	best := factMatch{}
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.Archived || !embeddingMergeEligible(*incoming, *candidate) {
			continue
		}
		if len(candidate.Vector) == 0 {
			vector, err := w.embedding.Vectorize(ctx, candidate.Content)
			if err != nil || len(vector) == 0 {
				continue
			}
			candidate.Vector = vector
			if err := w.store.UpdateFactEmbedding(ctx, candidate.ID, vector, factContentSHA256(candidate.Content), w.embeddingModelID); err != nil {
				continue
			}
		}
		similarity := cosineSimilarity(incomingVector, candidate.Vector)
		if similarity >= embeddingFactMergeThreshold && similarity > best.similarity {
			best = factMatch{fact: *candidate, reason: "embedding_semantic", similarity: similarity}
		}
	}
	if best.fact.ID == 0 {
		return nil
	}
	return &best
}

func factContentSHA256(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}

func embeddingMergeEligible(a, b types.FactEntry) bool {
	if !sameFactBucket(a, b) || hasNegativePolarity(a.Content) != hasNegativePolarity(b.Content) || !sameNumbers(a.Content, b.Content) {
		return false
	}
	lenA, lenB := len([]rune(compactSemanticText(a.Content))), len([]rune(compactSemanticText(b.Content)))
	if lenA == 0 || lenB == 0 {
		return false
	}
	return float64(minInt(lenA, lenB))/float64(maxInt(lenA, lenB)) >= 0.4
}

func compactSemanticText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasNegativePolarity(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range []string{"不喜欢", "讨厌", "不接受", "不要", "不能", "不会", "没有", "未", "禁止", "拒绝", "dislike", "hate", "not ", "never"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func sameNumbers(a, b string) bool {
	aNums, bNums := numberPattern.FindAllString(a, -1), numberPattern.FindAllString(b, -1)
	sort.Strings(aNums)
	sort.Strings(bNums)
	return strings.Join(aNums, "\x1f") == strings.Join(bNums, "\x1f")
}

func runeNGrams(s string, n int) map[string]struct{} {
	runes := []rune(s)
	out := make(map[string]struct{})
	if len(runes) < n {
		out[s] = struct{}{}
		return out
	}
	for i := 0; i <= len(runes)-n; i++ {
		out[string(runes[i:i+n])] = struct{}{}
	}
	return out
}

func setIntersectionSize(a, b map[string]struct{}) int {
	if len(a) > len(b) {
		a, b = b, a
	}
	n := 0
	for value := range a {
		if _, ok := b[value]; ok {
			n++
		}
	}
	return n
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
