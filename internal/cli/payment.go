package cli

import "strings"

// paymentRequiredMessage is what every command prints for the api's
// payment_required (exit 7): the api's message verbatim, because since
// DECISIONS I-289 it is the whole sentence for every reason
// (subscription_required, plan_limit, disk_limit, egress_limit, past_due,
// suspended), naming the machines using the memory or the date the period
// ends. An older api sends a fragment and the card-era reasons; those get
// the plan sentence.
// planName is a plan id as the name the dashboard shows (docs/PRICING.md).
func planName(id string) string {
	switch id {
	case "solo":
		return "Solo"
	case "pro":
		return "Pro"
	case "":
		return "your plan"
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

func paymentRequiredMessage(e *APIError) string {
	reason, _ := e.Detail["reason"].(string)
	switch reason {
	case "subscription_required", "plan_limit", "disk_limit", "egress_limit", "past_due", "suspended":
		if e.Message != "" {
			return e.Message
		}
	}
	return "Choose a plan at https://repose.herakraft.co/billing first."
}
