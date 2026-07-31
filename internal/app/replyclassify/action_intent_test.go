package replyclassify

import "testing"

func TestLexiconWrongPersonIsActionClassNotNegative(t *testing.T) {
	r := Classify(Input{Subject: "Re: intro", BodyText: "Thanks, but I am the wrong person for this."})
	if r.Class != ClassWrongPerson {
		t.Fatalf("Classify(wrong person) class = %q, want %q", r.Class, ClassWrongPerson)
	}
}

func TestLexiconBadTimingIsActionClass(t *testing.T) {
	r := Classify(Input{Subject: "Re: intro", BodyText: "This is not the right time, please circle back next quarter."})
	if r.Class != ClassBadTiming {
		t.Fatalf("Classify(bad timing) class = %q, want %q", r.Class, ClassBadTiming)
	}
}

func TestLexiconQuestionIsActionClass(t *testing.T) {
	r := Classify(Input{Subject: "Re: intro", BodyText: "Quick question — can you send pricing before we book time?"})
	if r.Class != ClassQuestion {
		t.Fatalf("Classify(question) class = %q, want %q", r.Class, ClassQuestion)
	}
}

func TestLexiconReferralIsActionClass(t *testing.T) {
	r := Classify(Input{Subject: "Re: intro", BodyText: "Please contact our finance lead Maya instead — looping her in here."})
	if r.Class != ClassReferral {
		t.Fatalf("Classify(referral) class = %q, want %q", r.Class, ClassReferral)
	}
}

func TestModelJSONSupportsActionClasses(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "question", raw: `{"class":"question","confidence":0.84}`, want: ClassQuestion},
		{name: "wrong person", raw: `{"class":"wrong_person","confidence":0.91}`, want: ClassWrongPerson},
		{name: "bad timing", raw: "```json\n{\"class\":\"bad_timing\",\"confidence\":0.78}\n```", want: ClassBadTiming},
		{name: "referral", raw: `{"class":"referral","confidence":0.88}`, want: ClassReferral},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, confidence := normalizeModelVerdict(tc.raw)
			if got != tc.want {
				t.Fatalf("normalizeModelVerdict class = %q, want %q", got, tc.want)
			}
			if confidence <= 0 || confidence > 1 {
				t.Fatalf("normalizeModelVerdict confidence = %v, want in (0,1]", confidence)
			}
		})
	}
}

func TestModelRejectsUnknownClasses(t *testing.T) {
	got, _ := normalizeModelVerdict(`{"class":"unsubscribe","confidence":0.99}`)
	if got != "" {
		t.Fatalf("normalizeModelVerdict(unsubscribe) class = %q, want empty", got)
	}
	got, _ = normalizeModelVerdict(`{"class":"questionable","confidence":0.99}`)
	if got != "" {
		t.Fatalf("normalizeModelVerdict(questionable) class = %q, want empty", got)
	}
}

func TestModelRejectsBareLabels(t *testing.T) {
	got, confidence := normalizeModelVerdict("wrong_person")
	if got != "" || confidence != 0 {
		t.Fatalf("normalizeModelVerdict(bare label) = (%q, %v), want empty/0", got, confidence)
	}
}
