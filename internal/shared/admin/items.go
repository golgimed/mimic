package admin

import (
	"sort"

	"github.com/golgimed/mimic/internal/registry"
)

// ListItems aggregates listItems() across every enabled provider, newest first.
func ListItems(reg *registry.Registry, enabledProviders []string) []registry.DashboardItem {
	items := []registry.DashboardItem{}
	for _, p := range reg.Enabled(enabledProviders) {
		if p.ListItems == nil {
			continue
		}
		items = append(items, p.ListItems()...)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items
}

func GetItemDetail(reg *registry.Registry, provider, id string) (*registry.ItemDetail, bool) {
	p, ok := reg.Get(provider)
	if !ok || p.GetItemDetail == nil {
		return nil, false
	}
	return p.GetItemDetail(id)
}
