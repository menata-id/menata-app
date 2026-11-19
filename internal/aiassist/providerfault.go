package aiassist

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// Fault is why a call to the model provider failed, said as what the person at the screen can do about it.
//
// Until 2026-11-19 every failure of the provider was shown as "Menata didn't get a response in time. Please try
// sending your message again." On 2026-10-25 the provider answered 402 (prepayment credits depleted) five times and a
// person sent the same request five times in 3.5 hours, because the screen told them to: five of the 22 unfinished
// creation sessions of that month. A retry cures a timeout and nothing else here, so the words differ by cause.
type Fault string

const (
	// FaultBilling: the account has no credit (402). Retrying cannot help; someone must top it up.
	FaultBilling Fault = "billing"
	// FaultAccess: the provider refused the key (401, 403). Retrying cannot help; the key must be checked.
	FaultAccess Fault = "access"
	// FaultQuota: over a usage limit (429). Retrying after a wait helps.
	FaultQuota Fault = "quota"
	// FaultTimeout: no answer in time, a network failure, or the provider's own server error (5xx). Retrying helps.
	FaultTimeout Fault = "timeout"
	// FaultOther: the provider answered and the answer was not usable (a malformed reply, a 4xx not named above).
	FaultOther Fault = "other"
)

// ProviderError is a non-200 answer from the model provider. Its Error text is what the plain Errorf it replaced said,
// so logs and anything reading the text are unchanged.
type ProviderError struct {
	StatusCode int
	// Status is the provider's status line ("402 Payment Required").
	Status string
	// Body is the start of the provider's answer.
	Body string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("gemini returned %s: %s", e.Status, e.Body)
}

// Kind classifies the status code.
func (e *ProviderError) Kind() Fault {
	switch {
	case e.StatusCode == http.StatusPaymentRequired:
		return FaultBilling
	case e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden:
		return FaultAccess
	case e.StatusCode == http.StatusTooManyRequests:
		return FaultQuota
	case e.StatusCode >= 500:
		return FaultTimeout
	}
	return FaultOther
}

// FaultOf classifies any error a Client returned: a ProviderError by its status, a deadline or network timeout as
// FaultTimeout, a failure to reach the provider at all as FaultTimeout (the person can try again), and anything
// else as FaultOther.
func FaultOf(err error) Fault {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Kind()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FaultTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return FaultTimeout
	}
	return FaultOther
}

// FaultStatus is the status code to log with a fault, or 0 when the provider never answered.
func FaultStatus(err error) int {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.StatusCode
	}
	return 0
}

// retryWords is the sentence a retry-able failure has always shown.
const retryWords = "Menata didn't get a response in time. Please try sending your message again."

// FaultMessage is what the conversation says when the model could not be reached. A cause a retry cannot cure says
// so, and says who can cure it.
func FaultMessage(f Fault) string {
	switch f {
	case FaultBilling:
		return "The AI assistant is unavailable: the provider reports that the account has no credit left. Sending the message again will not help -- whoever manages the Gemini account needs to add credit."
	case FaultAccess:
		return "The AI assistant is unavailable: the provider refused the service's key. Sending the message again will not help -- the Gemini key needs to be checked."
	case FaultQuota:
		return "The AI assistant has reached its usage limit for now. Wait a little, then send your message again."
	}
	return retryWords
}
