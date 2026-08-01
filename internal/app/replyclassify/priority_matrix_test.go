package replyclassify

import "testing"

func TestLexiconPriorityMatrix(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unsubscribe beats positive",
			body: "Sounds good, interested. Also unsubscribe me from further emails.",
			want: ClassUnsubscribe,
		},
		{
			name: "unsubscribe beats referral",
			body: "Please contact our ops lead instead, but remove me from this list.",
			want: ClassUnsubscribe,
		},
		{
			name: "unsubscribe beats wrong person",
			body: "I am not responsible for this. Please stop emailing me.",
			want: ClassUnsubscribe,
		},
		{
			name: "referral beats generic negative",
			body: "This is not the right fit for me. Please contact our finance lead Maya instead.",
			want: ClassReferral,
		},
		{
			name: "wrong person beats generic negative",
			body: "I am not responsible for procurement, so this is not the right contact.",
			want: ClassWrongPerson,
		},
		{
			name: "bad timing beats positive",
			body: "This sounds good, but not the right time. Circle back next quarter.",
			want: ClassBadTiming,
		},
		{
			name: "question beats positive",
			body: "Sounds good. Quick question, can you send pricing before we book a call?",
			want: ClassQuestion,
		},
		{
			name: "negative beats interested substring",
			body: "Not interested, no thanks.",
			want: ClassNegative,
		},
		{
			name: "positive clear interest",
			body: "Interested. Please send me more details and let's chat.",
			want: ClassPositive,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{Subject: "Re: outreach", BodyText: tc.body})
			if got.Class != tc.want {
				t.Fatalf("Classify(%q) class = %q, want %q (source=%q confidence=%v)", tc.body, got.Class, tc.want, got.Source, got.Confidence)
			}
			if got.Source != SourceLexicon {
				t.Fatalf("Classify(%q) source = %q, want %q", tc.body, got.Source, SourceLexicon)
			}
		})
	}
}

func TestHeaderAutomationShortCircuitsLexicon(t *testing.T) {
	got := Classify(Input{
		Headers:  map[string][]string{"Auto-Submitted": {"auto-replied"}},
		Subject:  "Re: outreach",
		BodyText: "Interested, please send pricing.",
	})
	if !IsAutomated(got.Class) {
		t.Fatalf("Classify(auto-replied interested body) class = %q, want an automated class", got.Class)
	}
	if got.Source != SourceHeader {
		t.Fatalf("Classify(auto-replied interested body) source = %q, want %q", got.Source, SourceHeader)
	}
}
