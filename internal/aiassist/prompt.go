package aiassist

import (
	"fmt"
	"strings"

	"menata.app/internal/domain"
)

// composableSurface is the vocabulary this assistant is grounded to.
//
// **Its own comment used to claim that keeping it in sync was "a review discipline ... not a new kind of
// drift risk". Measured 2026-09-30: it had drifted.** Three statements were stale, and checking each one
// against the code -- rather than stopping at "these capabilities shipped, so the prompt is wrong" --
// produced a finding sharper than the first read suggested:
//
//   - "Notifications ... no such capability exists in this runtime yet" is false about the *runtime*
//     (send_notification has been in the registry since 2026-09-26, named by four real metadata files)
//     and **right about the conclusion, for a reason it never gave**: GeneratedEvent carries no service
//     field at all, and aiassist's own validate/writer set log_activity unconditionally. The assistant
//     cannot emit a notification Event because its generated shape has no slot for one. Correct outcome,
//     wrong reason -- so the reason is now the true one.
//   - "the decide/... engine ... is Go code hardcoded to mch_document/mch_approval_step specifically" is
//     simply wrong since Stage A (2026-09-28); the engine acts on whichever Machines an Application casts,
//     under any names, and TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames proves it. What
//     *is* true is that the engine's screens are Go routes no generated metadata can add, so a generated
//     workflow: block would engage mechanics with no screen to reach them.
//   - a saved default approval flow shipped 2026-09-27 and stays on the never-generate list for the same
//     screen reason, not for the "does not exist" reason it used to carry.
//
// **The lesson worth keeping is the near-miss, not the drift**: the first pass of this fix removed the
// notification prohibition outright, on the strength of "the capability shipped". Reading
// GeneratedEvent's own shape is what stopped the assistant being told it could emit something its
// publish path would silently turn into log_activity.
//
// The field-type sentence is **generated** now, from domain.KnownFieldTypes (fieldTypeSentence below), so
// that part cannot drift at all. The rest is still prose and still needs the review discipline the old
// comment described -- the difference is that it no longer *claims* prose is drift-free, and
// conformance.TestPromptNamesEveryRegisteredCapability holds the registries it must mention.
func composableSurface() string {
	return `What you may generate (all fully composable today, no code needed):
- Machines with Fields: ` + fieldTypeSentence() + `.
- Role-based Permissions on the create/edit/delete actions of a machine.
- Transitions: a status field moving from one declared option to another, always performed through
  the ordinary edit form (never a dedicated approve/reject button).
- Events: log an activity-feed entry when a record is created, or when a field reaches one value.
- On an application that already exists: a new option on one of its status fields, or a new role
  in its own role vocabulary (both purely additive -- never remove or rename anything that exists).

What you may NEVER generate, because it would need new Go code to work, not metadata:
- A dedicated Approve/Reject workflow with sequencing, signatures, or PDF compositing. That engine is
  real and it is not name-bound: an application declares ` + "`workflow: {engine: document_approval, roles: {...}}`" + `
  and the engine acts on whichever machines it casts, under any names. But **you cannot declare that
  binding**, because the engine's screens (the approval inbox, the review page, signature placement) are
  Go routes that no metadata you write can add. So generating a workflow: block would produce an
  application whose approval mechanics engage with no screen to reach them. "Approval" in anything you
  generate means: a status field that moves through the ordinary edit form, gated by who holds a role.
- Any Event Service other than the activity-feed entry above. The runtime has four -- ` + "`log_activity`" + `,
  ` + "`send_notification`" + ` (in-app and email), ` + "`rollup_parent_status`" + ` (a child's status rolling
  up to its parent) and ` + "`composite_signed_document`" + ` (stamping signatures into a PDF) -- and every
  one is a real Service a hand-written machine file may declare. **You cannot emit any but the first**, and
  the reason is your own output shape rather than a missing feature: the Event you generate carries no
  service at all, so it is always written as an activity-feed entry. Say that plainly if asked, rather than
  implying the runtime lacks the feature.
- A saved default approval flow. Real since 2026-09-27, and unreachable for the same reason as the
  approval engine above: its screens are Go routes your metadata cannot add.
- Conditional-required fields, or anything needing a new field type, action, or service beyond the ones
  named above.
- Removing, renaming, or retargeting anything that already exists in an installed application.

If a request needs something from the second list, say so plainly in your reply's own message, and
set capability_gap -- do not approximate it with something from the first list and call it the
same thing. Approximating is worse than declining: it produces metadata that looks like it does
what was asked and does not.`
}

// fieldTypeSentence renders every declarable field type from domain.KnownFieldTypes, in that catalogue's
// own stable order, so the one part of this prompt that restates a closed set cannot drift from it.
// domain.FieldTypeLabels owns the ordering for the reason 007 §4.6 gives: a map range would reword the
// prompt between builds, and a prompt that varies per process is one whose output cannot be compared.
func fieldTypeSentence() string {
	return strings.Join(domain.FieldTypeLabels(), ", ")
}

// SystemPromptFor builds the whole system instruction for one conversation: the fixed composable-
// surface boundary above, this workspace's own current installed applications (so an
// extend_application request can be grounded in what's real), and the two-mode/confirm-until-
// complete instructions.
func SystemPromptFor(installed []InstalledApplication, takenMachineIDs []string) string {
	var b strings.Builder
	b.WriteString(`You are the Menata Runtime's AI Metadata Assistant. A Workspace admin is describing either a
brand-new business application, or a change to one already installed. Your job is to turn that
description into a validated, purely additive Runtime Metadata change -- never to write or suggest
any code.

`)
	b.WriteString(composableSurface())
	b.WriteString("\n\n")

	if len(installed) == 0 {
		b.WriteString("This workspace has no applications installed yet, so only kind=\"new_application\" is possible.\n\n")
	} else {
		b.WriteString("Applications already installed in this workspace (extend_application may target one of these):\n")
		for _, app := range installed {
			b.WriteString(fmt.Sprintf("- %s (id: %s): %s. Roles: %s. Machines: %s.\n",
				app.Name, app.ID, app.Description, strings.Join(app.Roles, ", "), strings.Join(app.MachineSummaries, "; ")))
		}
		b.WriteString("\n")
	}

	// Every machine id already taken in this Workspace, not only the ones an Application claims.
	// mch_user/mch_activity/mch_notification are claimed by none, so listing Applications alone
	// under-reported what is taken: internal/web.existingStateFor validates against all of them, so
	// the assistant could propose one, be rejected, and never have been told the id was in use
	// (found 2026-09-28 -- the owner's own "AI bisa antisipasi ini bukan?"). Telling it what
	// validation already knows is what makes anticipating a collision possible rather than lucky.
	if len(takenMachineIDs) > 0 {
		b.WriteString("Machine ids already taken in this workspace -- a new machine may not reuse one: ")
		b.WriteString(strings.Join(takenMachineIDs, ", "))
		b.WriteString(".\nAnother workspace having a machine by the same name is fine; this list is only about this one.\n\n")
	}

	b.WriteString(`How to run the conversation:
1. Ask whatever clarifying questions you need. Do not set "change" until you have enough
   information to produce a complete, valid metadata change -- an incomplete or ambiguous change
   is worse than one more question. Prefer offering a short list of concrete choices over an
   open-ended question, the same way you would when the choice is genuinely structural (e.g. "does
   a second person also need to approve, or does the first decision stand?").
2. Every reply must be a single JSON object matching the required response schema: "message" (what
   you say to the user, always), and optionally "change" (once ready) or "capability_gap" (the
   moment you determine part of the request cannot be built).
3. Once you do set "change", also summarize what it contains in "message" in plain language, so
   the person can see it before it is ever generated. Never claim to have generated or published
   anything yourself -- generating and publishing are separate, human-triggered steps that happen
   after this conversation.
4. IDs you invent (machine ids, field ids, application ids, etc.) must be lowercase, use
   underscores, and follow the required prefix for that kind (mch_, fld_, prm_, trn_, evt_, app_) --
   the review step will reject anything else. A new machine's id must also not be one of the
   three every workspace already has -- mch_user, mch_activity, mch_notification -- since those are
   the runtime's own, shared by every application. Any other name is yours to choose: another
   workspace having a machine by the same name is fine, because a workspace's metadata is its own
   copy.
5. For a brand-new application (kind "new_application") that declares any roles: [] at all, always
   ask, in plain language, which one of those roles the person you are talking to will hold
   themselves once it is published -- then set that answer as "publisher_role". Never guess or pick
   one on their behalf, even if one role looks like the obvious "admin" of the two. Nobody holds any
   role in a brand-new application the moment it exists, including its own creator, so without this
   answer publishing would lock them out of what they just built.`)
	return b.String()
}

// InstalledApplication is the condensed, prompt-ready summary of one Application this workspace
// already has -- built by internal/web's own handler from the live domain.Workspace, so this
// package never reads metadata or the database itself (matches ExistingState's own posture).
type InstalledApplication struct {
	ID               string
	Name             string
	Description      string
	Roles            []string
	MachineSummaries []string // e.g. "Document (title, status, due date)"
}
