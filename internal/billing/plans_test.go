package billing_test

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/billing"
)

// TestPlansMatchPricingDoc parses the plan table in docs/PRICING.md and
// fails when plans.go disagrees with it (PRICING.md "Changing prices":
// the numbers live in two places and nowhere else).
func TestPlansMatchPricingDoc(t *testing.T) {
	b, err := os.ReadFile("../../docs/PRICING.md")
	if err != nil {
		t.Fatal(err)
	}
	// | Solo | $29 a month | 8 GB: ... | 100 GB | 250 GB | 1 |
	row := regexp.MustCompile(`(?m)^\| (Solo|Pro) \| \$(\d+) a month \| (\d+) GB[^|]*\| (\d+) GB \| (\d+) GB \| (\d+) \|`)
	found := map[string]bool{}
	for _, m := range row.FindAllStringSubmatch(string(b), -1) {
		plan, ok := billing.PlanByID(strings.ToLower(m[1]))
		if !ok {
			t.Fatalf("PRICING.md names plan %q, plans.go has no such plan", m[1])
		}
		found[plan.ID] = true
		n := func(s string) int { v, _ := strconv.Atoi(s); return v }
		if plan.PriceCents != int64(n(m[2]))*100 || plan.MemoryGB != n(m[3]) || plan.DiskGB != n(m[4]) || plan.EgressGB != n(m[5]) || plan.Seats != n(m[6]) {
			t.Errorf("%s: PRICING.md says $%s, %s GB, %s GB disk, %s GB egress, %s seats; plans.go has %+v", plan.Name, m[2], m[3], m[4], m[5], m[6], plan)
		}
	}
	for _, p := range billing.Plans {
		if !found[p.ID] {
			t.Errorf("PRICING.md's table has no row for %s", p.Name)
		}
	}
	doc := string(b)
	for _, want := range []string{"$0.05 a GB", "10 on Solo, 25 on Pro", "four times the", "Seven days free"} {
		if !strings.Contains(doc, want) {
			t.Errorf("PRICING.md no longer says %q", want)
		}
	}
	if billing.OveragePerGBCents != 5 || billing.EgressHardStopMultiplier != 4 || billing.Solo.ProjectLimit != 10 || billing.Pro.ProjectLimit != 25 || billing.Solo.TrialDays != 7 {
		t.Errorf("plans.go constants drifted from PRICING.md")
	}
	if billing.PriceVersion != "plan-v1" {
		t.Errorf("PriceVersion %q", billing.PriceVersion)
	}
}

func TestOverageAndClassMemory(t *testing.T) {
	gb := int64(1) << 30
	for _, c := range []struct {
		plan  billing.Plan
		bytes int64
		gb    int64
		cents int64
	}{
		{billing.Solo, 20 * gb, 0, 0},
		{billing.Solo, 250 * gb, 0, 0},
		{billing.Solo, 250*gb + 1, 1, 5},
		{billing.Solo, 300 * gb, 50, 250},
		{billing.Pro, 500 * gb, 0, 0},
		{billing.Pro, 600*gb + gb/2, 101, 505},
	} {
		g, cents := billing.OverageCents(c.plan, c.bytes)
		if g != c.gb || cents != c.cents {
			t.Errorf("%s %d bytes: %d GB %d cents, want %d GB %d cents", c.plan.ID, c.bytes, g, cents, c.gb, c.cents)
		}
	}
	if billing.ClassMemoryGB("small") != 4 || billing.ClassMemoryGB("large") != 8 || billing.ClassMemoryGB("xl") != 16 {
		t.Error("class memory drifted from docs/interfaces/README.md")
	}
	if !billing.Pro.AllowsXL() || billing.Solo.AllowsXL() {
		t.Error("xl needs 16 GB: Pro only")
	}
	if billing.Solo.EgressHardStopBytes() != 1000*gb || billing.Pro.EgressHardStopBytes() != 2000*gb {
		t.Error("the hard stop is 1 TB on Solo, 2 TB on Pro")
	}
	r := billing.Price(billing.Inputs{Class: "large", RunningSeconds: 7200, GBAlloc: 40, EgressBytes: -5})
	if r.RunningSeconds != 3600 || r.EgressBytes != 0 || r.CostCents != 0 || r.EgressCents != 0 || r.GBAlloc != 40 {
		t.Errorf("Price normalises and prices nothing: %+v", r)
	}
}
