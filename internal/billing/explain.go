package billing

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/db"
)

// `repose-admin billing explain <project> <hour>` prints every input and
// each step of §5.4 for one usage_hours row. It is the tool for answering
// a support ticket, so it shows the arithmetic rather than the answer:
// what was sampled, what the period running totals were before the hour,
// which cap applied and why, and what reached Stripe.

// Explanation is one priced hour, taken apart.
type Explanation struct {
	ProjectID   uuid.UUID
	Slug        string
	Handle      string
	Hour        time.Time
	Class       string
	CapClass    string
	Period      Period
	PriceVersio string

	RunningSeconds int
	SampleMinutes  int
	Gap            bool
	GBAlloc        int64
	EgressBytes    int64

	// Period running totals before this hour.
	PriorGuestCents  int64
	PriorEgressBytes int64
	PriorRemainder   int64

	GuestCents       int64
	StorageCents     int64
	EgressCents      int64
	CostCents        int64
	CreditCents      int64
	StorageRemainder int64
	CapCents         int64
	CapApplied       bool

	StripeRecordID string
}

// Explain reads one usage_hours row and reconstructs how it was priced.
func Explain(ctx context.Context, pool *db.Pool, projectID uuid.UUID, hour time.Time) (Explanation, error) {
	h := hour.UTC().Truncate(time.Hour)
	e := Explanation{ProjectID: projectID, Hour: h}
	var periodStart, periodEnd *time.Time
	var recordID *string
	err := pool.QueryRow(ctx, `select p.slug, u.handle, h.class, h.running_seconds, h.gb_alloc, h.egress_bytes, h.gap,
		h.guest_cents, h.storage_cents, h.egress_cents, h.cost_cents, h.credit_cents, h.storage_remainder,
		h.period_start, h.period_end, h.price_version, h.stripe_usage_record_id
		from usage_hours h join projects p on p.id = h.project_id join users u on u.id = p.user_id
		where h.project_id = $1 and h.hour = $2`, projectID, h).
		Scan(&e.Slug, &e.Handle, &e.Class, &e.RunningSeconds, &e.GBAlloc, &e.EgressBytes, &e.Gap,
			&e.GuestCents, &e.StorageCents, &e.EgressCents, &e.CostCents, &e.CreditCents, &e.StorageRemainder,
			&periodStart, &periodEnd, &e.PriceVersio, &recordID)
	if err != nil {
		return e, fmt.Errorf("read usage_hours for %s at %s: %w", projectID, h.Format(time.RFC3339), err)
	}
	if periodStart != nil {
		e.Period.Start = periodStart.UTC()
	}
	if periodEnd != nil {
		e.Period.End = periodEnd.UTC()
	}
	if recordID != nil {
		e.StripeRecordID = *recordID
	}
	// The samples the hour was built from, for the gap line.
	if err := pool.QueryRow(ctx, "select count(*) from meter_samples where project_id = $1 and ts >= $2 and ts < $3 and state = 'running'",
		projectID, h, h.Add(time.Hour)).Scan(&e.SampleMinutes); err != nil {
		return e, fmt.Errorf("count the hour's samples: %w", err)
	}
	// The period running totals as they were before this hour, and the
	// largest class the period has seen, which is the cap that applied.
	var capClass *string
	err = pool.QueryRow(ctx, `select coalesce(sum(guest_cents), 0), coalesce(sum(egress_bytes), 0),
		coalesce((select storage_remainder from usage_hours where project_id = $1 and period_start = $2 and hour < $3 order by hour desc limit 1), 0),
		(select class from usage_hours where project_id = $1 and period_start = $2 and hour <= $3
		 order by case class when 'xl' then 3 when 'large' then 2 when 'small' then 1 else 0 end desc limit 1)
		from usage_hours where project_id = $1 and period_start = $2 and hour < $3`,
		projectID, e.Period.Start, h).Scan(&e.PriorGuestCents, &e.PriorEgressBytes, &e.PriorRemainder, &capClass)
	if err != nil {
		return e, fmt.Errorf("read the period running totals: %w", err)
	}
	if capClass != nil {
		e.CapClass = *capClass
	} else {
		e.CapClass = e.Class
	}
	e.CapCents = Cap(e.CapClass)
	e.CapApplied = e.PriorGuestCents+uncappedGuest(e.Class, e.RunningSeconds) > e.CapCents
	return e, nil
}

// uncappedGuest is the guest part before the cap, which is what shows the
// cap doing something.
func uncappedGuest(class string, runningSeconds int) int64 {
	secs := int64(runningSeconds)
	if secs > 3600 {
		secs = 3600
	}
	return (Hourly(class)*secs + 1800) / 3600
}

// WriteTo prints the explanation as the operator reads it.
func (e Explanation) WriteTo(w io.Writer) (int64, error) {
	var b strings.Builder
	f := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	f("project      %s (%s, owner %s)\n", e.Slug, e.ProjectID, e.Handle)
	f("hour         %s  price version %s\n", e.Hour.Format(time.RFC3339), e.PriceVersio)
	f("period       %s .. %s  (%d hours)\n", e.Period.Start.Format(time.RFC3339), e.Period.End.Format(time.RFC3339), e.Period.Hours())
	f("class        %s at %d cents/hour; period cap class %s at %d cents\n", e.Class, Hourly(e.Class), e.CapClass, e.CapCents)
	f("\ninputs (5.4)\n")
	f("  running    %d s from %d running samples (a sample is a minute)%s\n", e.RunningSeconds, e.SampleMinutes, gapNote(e.Gap))
	f("  disk       %d GB allocated\n", e.GBAlloc)
	f("  egress     %d bytes this hour, %d bytes earlier this period\n", e.EgressBytes, e.PriorEgressBytes)
	f("\nsteps\n")
	f("  guest      %d * %d / 3600, rounded half up = %d cents\n", Hourly(e.Class), e.RunningSeconds, uncappedGuest(e.Class, e.RunningSeconds))
	if e.CapApplied {
		f("             period guest total was %d cents, cap is %d, so this hour is clipped to %d\n", e.PriorGuestCents, e.CapCents, e.GuestCents)
	} else {
		f("             period guest total %d + %d = %d, under the %d cent cap\n", e.PriorGuestCents, e.GuestCents, e.PriorGuestCents+e.GuestCents, e.CapCents)
	}
	f("  storage    %d GB * %d cents / %d hours = %d cents, remainder carried %d/%d\n",
		e.GBAlloc, StoragePerGBMonth, e.Period.Hours(), e.StorageCents, e.StorageRemainder, 1000*int64(e.Period.Hours()))
	f("  egress     %d GB included per period; %d cents\n", EgressIncludedGB, e.EgressCents)
	f("  cost       %d + %d + %d = %d cents\n", e.GuestCents, e.StorageCents, e.EgressCents, e.CostCents)
	f("  credit     %d cents taken from the trial ledger\n", e.CreditCents)
	f("  billable   %d cents\n", e.CostCents-e.CreditCents)
	f("  stripe     %s\n", recordNote(e.StripeRecordID))
	n, err := io.WriteString(w, b.String())
	return int64(n), err
}

func gapNote(gap bool) string {
	if gap {
		return "  [GAP: the project was running and no sample arrived; the minutes are under-billed, never estimated]"
	}
	return ""
}

func recordNote(id string) string {
	switch id {
	case "":
		return "not pushed yet"
	case "exempt":
		return "not pushed: the account is billing-exempt (DECISIONS I-16)"
	}
	return id
}
