package metadata

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// decodeStrict is the one way this package turns YAML into a document, and it **rejects a key the
// runtime has no home for** (yaml.Decoder.KnownFields) instead of dropping it in silence.
//
// That silence was this runtime's last validation hole, and the only one where a wrong file looks
// exactly like a right one. Everything the loader *knows* is validated strictly -- an unknown field
// type, a dangling relation target, a `cards` View with no `card_fields`, an engine it cannot realize
// -- so a *misspelled or retired* key was the one mistake that produced no error anywhere: the Machine
// loaded clean and simply lacked the capability. Proved on 2026-09-20 by loading a copy of metadata/
// whose mch_document used the pre-2026-09-20 singular `view:` block: no error, zero Views. 005 Phase 3
// says invalid metadata must not reach compilation, and an unknown key is invalid for the binary
// reading it.
//
// **Who this actually protects**, checked rather than assumed: hand-written metadata, the template
// library, and any future writer of YAML. It is *not* the AI assistant's own publish path -- that
// marshals from typed Go structs (internal/aiassist's own machineDoc/applicationDoc) and appends list
// items to existing files, so it has never been able to emit a key at all, let alone an unknown one.
// The error wording below still matters there: aiassist.Write load-verifies through LoadApplication and
// hands any failure back to the conversation (internal/web's returnToConversation), so a message naming
// a Go type would be worse than noise to the one reader who cannot look at the source.
//
// **Map keys are not affected**, which is load-bearing for `workflow.roles`: KnownFields constrains
// struct fields, so `document: mch_document` under a map[string]string stays exactly as legal as it
// was. TestDecodeStrict_acceptsMapKeys holds that, because it is the property a later reader would most
// plausibly "fix".
func decodeStrict[T any](data []byte, doc *T, what string) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(doc); err != nil {
		return readableKeyError(err, what)
	}
	return nil
}

// unknownFieldMessage matches yaml.v3's own wording for a rejected key. The Go type name in it is
// noise to the person who wrote the file, and worse than noise on the assistant's path, where this
// message is fed straight back into a conversation as the thing to correct.
var unknownFieldMessage = regexp.MustCompile(`field ([a-zA-Z0-9_]+) not found in type [a-zA-Z0-9_.\[\]]+`)

// readableKeyError rewrites that message into one a metadata author can act on, keeping yaml's own
// line number -- which is the genuinely useful half and the reason this rewrites rather than replaces.
func readableKeyError(err error, what string) error {
	msg := unknownFieldMessage.ReplaceAllString(err.Error(),
		fmt.Sprintf("%q is not a key %s declares", "$1", what))
	if strings.Contains(msg, "is not a key") {
		msg += "\n    (see writing-guide.md §12 for every key this runtime reads; a retired or misspelled one used to be ignored, which is why it is refused here)"
	}
	return fmt.Errorf("%s", msg)
}
