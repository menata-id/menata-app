package aiassist

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func contractRequest() ExtractRequest {
	return ExtractRequest{
		Instruction: "A production contract.",
		Document:    []byte("%PDF-1.4 fake"),
		MIME:        "application/pdf",
		Fields: []ExtractField{
			{ID: "fld_party", Label: "Contracted party", Type: "text"},
			{ID: "fld_kind", Label: "Type", Type: "choice", Options: []string{"Cast", "Crew"}},
		},
		Rows: []ExtractRowsQuestion{{Key: "terms", Label: "Instalments", Fields: []ExtractField{
			{ID: "fld_label", Label: "Label", Type: "text"}, {ID: "fld_gross", Label: "Gross", Type: "number"}}}},
	}
}

// TestTheReadingAsksForExactlyTheFieldsItWasGiven: the schema has a property per Field, per row kind an array of rows, and
// every object says the order it is written in (a model writes forward only).
func TestTheReadingAsksForExactlyTheFieldsItWasGiven(t *testing.T) {
	body := newExtractBody(contractRequest())
	var check func(path string, s geminiSchema)
	check = func(path string, s geminiSchema) {
		if s.Type == "OBJECT" && len(s.PropertyOrdering) != len(s.Properties) {
			t.Errorf("%s: %d properties but an ordering of %d", path, len(s.Properties), len(s.PropertyOrdering))
		}
		for k, v := range s.Properties {
			check(path+"."+k, v)
		}
		if s.Items != nil {
			check(path+"[]", *s.Items)
		}
	}
	root := body.GenerationConfig.ResponseSchema
	check("root", root)
	if _, ok := root.Properties["values"].Properties["fld_kind"]; !ok {
		t.Errorf("no property for fld_kind: %v", root.Properties["values"])
	}
	if got := root.Properties["rows"].Properties["rows_0"]; got.Type != "ARRAY" || got.Items.Properties["fld_gross"].Type == "" {
		t.Errorf("rows_0 = %+v", got)
	}
	parts := body.Contents[0].Parts
	if parts[0].InlineData == nil || parts[0].InlineData.MIMEType != "application/pdf" {
		t.Fatalf("the document is not sent inline: %+v", parts[0])
	}
	if raw, _ := base64.StdEncoding.DecodeString(parts[0].InlineData.Data); string(raw) != "%PDF-1.4 fake" {
		t.Errorf("the document arrived as %q", raw)
	}
	for _, want := range []string{"fld_kind: Type (choice) -- one of: Cast | Crew", "rows_0: Instalments", "A production contract."} {
		if !strings.Contains(parts[1].Text, want) {
			t.Errorf("the question lacks %q:\n%s", want, parts[1].Text)
		}
	}
}

// TestADecodedReadingKeepsOnlyWhatWasAsked: an answer for a Field nobody asked about is dropped.
func TestADecodedReadingKeepsOnlyWhatWasAsked(t *testing.T) {
	text := `{"values":{"fld_party":{"source":"p. 1","value":"Reza","inferred":false},"fld_secret":{"source":"","value":"x","inferred":false}},
	  "rows":{"rows_0":[{"fld_label":{"source":"p. 3","value":"Signing","inferred":false},"fld_extra":{"value":"y"}}],"rows_9":[{"fld_label":{"value":"z"}}]}}`
	got, err := decodeExtract(text, contractRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Values["fld_secret"]; ok || got.Values["fld_party"].Value != "Reza" {
		t.Errorf("values = %+v", got.Values)
	}
	if rows := got.Rows["terms"]; len(rows) != 1 || rows[0]["fld_label"].Source != "p. 3" || len(rows[0]) != 1 {
		t.Errorf("rows = %+v", got.Rows)
	}
	if len(got.Rows) != 1 {
		t.Errorf("a row kind nobody asked for appeared: %+v", got.Rows)
	}
	if _, err := decodeExtract("not json", contractRequest()); err == nil {
		t.Error("a malformed answer was accepted")
	}
}

// TestTheGeminiExtractorSendsTheDocumentAndDecodesTheAnswer is the whole call against a server standing in for Google.
func TestTheGeminiExtractorSendsTheDocumentAndDecodesTheAnswer(t *testing.T) {
	var sentKey string
	var sent extractRequestBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sentKey = r.Header.Get("x-goog-api-key")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &sent)
		answer := `{"values":{"fld_party":{"source":"p. 1 · title","value":"Reza Anggara","inferred":false}},"rows":{"rows_0":[]}}`
		_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": answer}}}}}})
	}))
	defer srv.Close()
	got, err := GeminiExtractor{APIKey: "k", HTTPClient: srv.Client(), Endpoint: srv.URL}.Extract(context.Background(), contractRequest())
	if err != nil {
		t.Fatal(err)
	}
	if sentKey != "k" || sent.Contents[0].Parts[0].InlineData == nil {
		t.Errorf("key %q, document sent: %v", sentKey, sent.Contents[0].Parts[0].InlineData != nil)
	}
	if got.Values["fld_party"].Value != "Reza Anggara" {
		t.Errorf("values = %+v", got.Values)
	}
	if _, err := (GeminiExtractor{APIKey: "k", HTTPClient: srv.Client(), Endpoint: srv.URL}).Extract(context.Background(), ExtractRequest{Document: make([]byte, maxDocumentBytes+1)}); err == nil {
		t.Error("a document over the limit was sent")
	}
}

func TestAnUnconfiguredReaderIsNotAvailable(t *testing.T) {
	if Available(UnconfiguredExtractor{}) || Available(nil) || !Available(GeminiExtractor{}) {
		t.Error("Available does not tell a configured reader from none")
	}
	if _, err := (UnconfiguredExtractor{}).Extract(context.Background(), ExtractRequest{}); err != ErrExtractorNotConfigured {
		t.Errorf("got %v", err)
	}
}
