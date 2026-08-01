package replyclassify

import "strings"

// classifyLexicon is Layer 2: a deterministic, offline keyword scan over the
// subject + body. It returns (result, true) only on a CLEAR signal; ambiguous
// text returns (zero, false) so the optional model layer (or "unknown") decides.
//
// Order matters and encodes priority:
//  1. Compliance words (unsubscribe / stop / remove me / take me off) ALWAYS win.
//     Treating these as anything other than an unsubscribe request is a
//     compliance risk, so they short-circuit before action routing or sentiment.
//  2. Action-specific human replies route to referral, wrong_person, bad_timing,
//     or question before generic sentiment.
//  3. Clear rejection phrases => negative.
//  4. Clear interest phrases => positive.
func classifyLexicon(in Input) (Result, bool) {
	text := strings.ToLower(strings.TrimSpace(in.Subject + "\n" + in.BodyText))
	if text == "" {
		return Result{}, false
	}

	// 1. Compliance / opt-out (highest priority).
	for _, kw := range unsubscribeKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassUnsubscribe, Confidence: 0.9, Source: SourceLexicon}, true
		}
	}

	// 2. Action-specific human replies that should not collapse into negative.
	for _, kw := range referralKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassReferral, Confidence: 0.82, Source: SourceLexicon}, true
		}
	}
	for _, kw := range wrongPersonKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassWrongPerson, Confidence: 0.82, Source: SourceLexicon}, true
		}
	}
	for _, kw := range badTimingKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassBadTiming, Confidence: 0.8, Source: SourceLexicon}, true
		}
	}
	for _, kw := range questionKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassQuestion, Confidence: 0.78, Source: SourceLexicon}, true
		}
	}

	// 3. Clear rejection => negative. Evaluate this before positive so
	// phrases such as "not interested" are not misread by the positive
	// substring "interested". Compliance/opt-out still wins above.
	for _, kw := range negativeKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassNegative, Confidence: 0.8, Source: SourceLexicon}, true
		}
	}

	// 4. Clear interest => positive.
	for _, kw := range positiveKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassPositive, Confidence: 0.8, Source: SourceLexicon}, true
		}
	}

	return Result{}, false
}

// unsubscribeKeywords are explicit opt-out requests. Compliance-first: any of
// these short-circuits to "unsubscribe" before sentiment is considered.
var unsubscribeKeywords = []string{
	"unsubscribe",
	"opt out",
	"opt-out",
	"remove me",
	"take me off",
	"stop emailing",
	"stop contacting",
	"do not contact",
	"don't contact",
	"do not email",
	"don't email",
	"please stop",
}

// positiveKeywords are clear buying / interest signals. Kept conservative so the
// deterministic layer only fires on unambiguous intent; nuance is left to the
// model layer.
var positiveKeywords = []string{
	"interested",
	"sounds good",
	"sounds great",
	"let's chat",
	"lets chat",
	"let's talk",
	"lets talk",
	"happy to chat",
	"happy to talk",
	"set up a call",
	"book a call",
	"schedule a call",
	"schedule a demo",
	"book a demo",
	"send me more",
	"tell me more",
	"would love to",
	"count me in",
	"sign me up",
}

var questionKeywords = []string{
	"how much does it cost",
	"what's the pricing",
	"whats the pricing",
	"send pricing",
	"what is the pricing",
	"can you send pricing",
	"can you share pricing",
	"what does it cost",
	"how does it work",
	"can you clarify",
	"quick question",
}

var referralKeywords = []string{
	"please contact",
	"contact my colleague",
	"contact our",
	"reach out to my colleague",
	"reach out to our",
	"forwarded to",
	"looping in",
	"i've cc'd",
	"i have cc'd",
	"cc'd ",
	"cced ",
}

var wrongPersonKeywords = []string{
	"wrong person",
	"wrong contact",
	"not the right person",
	"not the best person",
	"not my area",
	"not my department",
	"i don't handle",
	"i do not handle",
	"i'm not responsible",
	"i am not responsible",
	"not responsible for",
}

var badTimingKeywords = []string{
	"not the right time",
	"bad timing",
	"too early",
	"circle back",
	"come back to me",
	"reach back out",
	"check back",
	"later this year",
	"next quarter",
	"next month",
	"in q1",
	"in q2",
	"in q3",
	"in q4",
}

// negativeKeywords are clear rejection signals. "not interested" is the canonical
// cold-outreach brush-off.
var negativeKeywords = []string{
	"not interested",
	"no thanks",
	"no thank you",
	"not a fit",
	"not the right",
	"not relevant",
	"no need",
	"we already have",
	"we have a solution",
	"not looking",
	"please don't",
	"leave me alone",
	"go away",
}
