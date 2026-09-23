package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// largeInventorySize is the number of items the performance test seeds: two
// orders of magnitude above a real home inventory, so a search that turns into
// one query per item (or a quadratic scan) shows up straight away.
const largeInventorySize = 10_000

// searchBudget is deliberately loose. The test is not a benchmark — it guards
// against an order-of-magnitude regression on a loaded CI machine, while the
// real timings are reported with t.Log so a slow query stays visible.
const searchBudget = 2 * time.Second

// The inventory mixes brands and kinds, so a query can be broad ("bosch
// drill"), narrow (a tag) or hopeless (a term nobody carries).
var (
	brands = []string{"Bosch", "Makita", "DeWalt", "Ryobi", "Einhell"}
	kinds  = []string{"drill", "hammer", "screwdriver", "saw", "clamp", "level", "tape", "trowel"}
)

// largeInventory fills a fresh database with n items spread over a room and
// nested containers, with a realistic mix of aliases, tags and descriptions.
func largeInventory(tb testing.TB, n int) Repos {
	tb.Helper()
	repos := NewRepos(openDB(tb, mustMigrate(tb)))
	ctx := context.Background()

	garage := inventory.Room{Name: "Garage"}
	if err := repos.Rooms.Create(ctx, &garage); err != nil {
		tb.Fatalf("creating room: %v", err)
	}
	toolbox := inventory.Container{RoomID: garage.ID, Name: "Toolbox"}
	if err := repos.Containers.Create(ctx, &toolbox); err != nil {
		tb.Fatalf("creating container: %v", err)
	}
	drawer := inventory.Container{RoomID: garage.ID, ParentID: &toolbox.ID, Name: "Drawer 1"}
	if err := repos.Containers.Create(ctx, &drawer); err != nil {
		tb.Fatalf("creating nested container: %v", err)
	}

	locations := []inventory.Location{
		inventory.RoomLocation(garage.ID),
		inventory.ContainerLocation(toolbox.ID),
		inventory.ContainerLocation(drawer.ID),
	}
	for i := range n {
		item := inventory.Item{
			Name:        fmt.Sprintf("%s %s %04d", brands[i%len(brands)], kinds[i%len(kinds)], i),
			Description: fmt.Sprintf("tool %d of the workshop", i),
			Quantity:    i%3 + 1,
			Location:    locations[i%len(locations)],
		}
		// A slice of the inventory carries the labels search also looks at.
		if i%10 == 0 {
			item.Aliases = []string{fmt.Sprintf("alias-%04d", i)}
		}
		if i%7 == 0 {
			item.Tags = []string{"officina", kinds[i%len(kinds)]}
		}
		if err := repos.Items.Create(ctx, &item); err != nil {
			tb.Fatalf("creating item %d: %v", i, err)
		}
	}
	return repos
}

// TestSearchPerformanceOnLargeInventory searches a seeded inventory the size
// of a small shop and checks both the answers and the time they take.
func TestSearchPerformanceOnLargeInventory(t *testing.T) {
	repos := largeInventory(t, largeInventorySize)
	ctx := context.Background()

	// "Bosch" is one brand in five and "drill" one kind in eight, so both
	// together select every fortieth item.
	multiTerm := (largeInventorySize + len(brands)*len(kinds) - 1) / (len(brands) * len(kinds))
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"multi-term", "bosch drill", multiTerm},
		{"alias", "alias-0500", 1},
		{"tag", "officina", (largeInventorySize + 6) / 7},
		// Nothing matches: the scan still has to read every item.
		{"no match", "cemento", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			matches, err := repos.Items.Search(ctx, tc.query)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("Search(%q): %v", tc.query, err)
			}
			if len(matches) != tc.want {
				t.Fatalf("Search(%q) over %d items = %d matches, want %d", tc.query, largeInventorySize, len(matches), tc.want)
			}
			t.Logf("Search(%q) over %d items: %v, %d matches", tc.query, largeInventorySize, elapsed.Round(time.Millisecond), len(matches))
			if elapsed > searchBudget {
				t.Fatalf("Search(%q) took %v over %d items, past the %v budget", tc.query, elapsed, largeInventorySize, searchBudget)
			}
		})
	}
}

// BenchmarkSearch measures the same queries without the assertion, so the
// numbers can be compared between changes.
func BenchmarkSearch(b *testing.B) {
	repos := largeInventory(b, largeInventorySize)
	ctx := context.Background()
	queries := []string{"bosch drill", "alias-0500", "officina", "cemento"}

	b.ResetTimer()
	for i := range b.N {
		if _, err := repos.Items.Search(ctx, queries[i%len(queries)]); err != nil {
			b.Fatalf("Search: %v", err)
		}
	}
}
