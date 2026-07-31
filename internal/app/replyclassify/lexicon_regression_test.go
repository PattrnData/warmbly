package replyclassify

import "testing"

func TestLexiconNegativeInterestPhraseWinsOverInterestedSubstring(t *testing.T) {
	r := Classify(Input{Subject: "Re: proof", BodyText: "Not interested, thanks. Please do not follow up."})
	if r.Class != ClassNegative {
		t.Fatalf("Classify(not interested) class = %q, want %q (source=%q confidence=%v)", r.Class, ClassNegative, r.Source, r.Confidence)
	}
	if r.Source != SourceLexicon {
		t.Fatalf("Classify(not interested) source = %q, want %q", r.Source, SourceLexicon)
	}
}

func TestLexiconPositiveInterestStillClassifiesPositive(t *testing.T) {
	r := Classify(Input{Subject: "Re: proof", BodyText: "Sounds good, interested — send more details."})
	if r.Class != ClassPositive {
		t.Fatalf("Classify(positive interest) class = %q, want %q", r.Class, ClassPositive)
	}
}

func TestLexiconUnsubscribeStillHasHighestPriority(t *testing.T) {
	r := Classify(Input{Subject: "Re: proof", BodyText: "Sounds good but unsubscribe me please."})
	if r.Class != ClassUnsubscribe {
		t.Fatalf("Classify(unsubscribe) class = %q, want %q", r.Class, ClassUnsubscribe)
	}
}
