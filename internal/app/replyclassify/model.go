package replyclassify

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// Layer 3: the OPTIONAL model classifier. It rides the platform LLM provider
// (M1) wired in from the app mains via SetModelClassifier, so it uses the same
// Warmbly AI backend as every other AI feature. It is
// platform-paid: this path never charges org credits (it settles only the
// ambiguous sentiment middle the cheap deterministic layers can't). When no
// provider is wired (no AI_PROVIDER) the layer is a pure
// no-op that resolves the middle to "unknown" WITHOUT any network call.
//
// The model is constrained to human-action classes. Compliance (unsubscribe)
// and automation (auto_reply / out_of_office) are already settled upstream and
// are intentionally NOT in the model's output space.

// modelTimeout bounds the single Layer-3 completion. The shared Ollama 9B
// classifier can take longer than small hosted models, especially while warming.
const modelTimeout = 30 * time.Second

const modelSystemPrompt = "Classify the latest human reply to a cold sales email into one action label. " +
	"Return strict JSON only: {\"class\":\"positive|negative|question|wrong_person|bad_timing|referral|neutral\",\"confidence\":0.0}. " +
	"Use positive for interest in a call/demo/more info, negative for a hard no/not interested, question for pricing/details/clarification, " +
	"wrong_person when the recipient says they are not responsible or not the right contact, bad_timing for not now/circle back later, " +
	"referral when they name or copy someone else to contact, neutral when unclear. Do not classify quoted prior email text."

// ModelClassifyFunc runs one platform LLM completion for Layer 3: given the
// system + user prompt it returns the model's raw text. The app mains adapt
// generation.Provider.Complete to this shape and wire it with SetModelClassifier,
// keeping this low-level package free of a direct provider dependency. nil means
// Layer 3 is disabled (the ambiguous middle resolves to "unknown" offline).
type ModelClassifyFunc func(ctx context.Context, system, user string) (string, error)

var (
	modelMu       sync.RWMutex
	modelClassify ModelClassifyFunc
)

// SetModelClassifier wires (or clears, with nil) the platform provider that
// backs Layer 3. Safe to call once at startup; guarded for concurrent reads.
func SetModelClassifier(fn ModelClassifyFunc) {
	modelMu.Lock()
	modelClassify = fn
	modelMu.Unlock()
}

// classifyModel runs Layer 3 when a provider is wired. Returns (zero, false)
// when unconfigured or on any error, so the caller falls back to "unknown" and
// NEVER hard-errors on a classification miss.
func classifyModel(ctx context.Context, in Input) (Result, bool) {
	modelMu.RLock()
	fn := modelClassify
	modelMu.RUnlock()
	if fn == nil {
		return Result{}, false
	}

	user := strings.TrimSpace("Subject: " + in.Subject + "\n\n" + in.BodyText)
	if user == "" {
		return Result{}, false
	}

	cctx, cancel := context.WithTimeout(ctx, modelTimeout)
	defer cancel()

	out, err := fn(cctx, modelSystemPrompt, user)
	if err != nil {
		return Result{}, false
	}

	class, confidence := normalizeModelVerdict(out)
	switch class {
	case ClassPositive:
		return Result{Class: ClassPositive, Confidence: confidence, Source: SourceModel}, true
	case ClassNegative:
		return Result{Class: ClassNegative, Confidence: confidence, Source: SourceModel}, true
	case ClassQuestion:
		return Result{Class: ClassQuestion, Confidence: confidence, Source: SourceModel}, true
	case ClassWrongPerson:
		return Result{Class: ClassWrongPerson, Confidence: confidence, Source: SourceModel}, true
	case ClassBadTiming:
		return Result{Class: ClassBadTiming, Confidence: confidence, Source: SourceModel}, true
	case ClassReferral:
		return Result{Class: ClassReferral, Confidence: confidence, Source: SourceModel}, true
	case ClassNeutral:
		return Result{Class: ClassNeutral, Confidence: confidence, Source: SourceModel}, true
	default:
		return Result{}, false
	}
}

type modelVerdict struct {
	Class      string  `json:"class"`
	Confidence float64 `json:"confidence"`
}

// normalizeModelVerdict reduces a strict-JSON model response to an allowlisted
// label. A code-fenced JSON block is tolerated because several
// OpenAI-compatible/local backends still wrap JSON despite the prompt, but bare
// labels are rejected: the classifier contract is JSON-only so a provider drift
// cannot silently widen the output shape.
func normalizeModelVerdict(s string) (string, float64) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	var v modelVerdict
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return normalizeAllowedClass(v.Class), normalizeConfidence(v.Confidence)
	}
	return "", 0
}

func normalizeAllowedClass(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, ".\"' \n	")
	switch s {
	case ClassPositive:
		return ClassPositive
	case ClassNegative:
		return ClassNegative
	case ClassQuestion:
		return ClassQuestion
	case ClassWrongPerson:
		return ClassWrongPerson
	case ClassBadTiming:
		return ClassBadTiming
	case ClassReferral:
		return ClassReferral
	case ClassNeutral:
		return ClassNeutral
	default:
		return ""
	}
}

func normalizeConfidence(v float64) float64 {
	if v <= 0 || v > 1 {
		return 0.7
	}
	return v
}
