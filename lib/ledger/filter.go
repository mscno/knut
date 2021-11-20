package ledger

import (
	"regexp"

	"github.com/sboehler/knut/lib/model/accounts"
	"github.com/sboehler/knut/lib/model/commodities"
)

// Filter represents a filter creating a ledger.
type Filter struct {
	AccountsFilter, CommoditiesFilter *regexp.Regexp
}

// MatchAccount returns whether this filter matches the given Account.
func (b Filter) MatchAccount(a *accounts.Account) bool {
	return b.AccountsFilter == nil || b.AccountsFilter.MatchString(a.String())
}

// MatchCommodity returns whether this filter matches the given Commodity.
func (b Filter) MatchCommodity(c *commodities.Commodity) bool {
	return b.CommoditiesFilter == nil || b.CommoditiesFilter.MatchString(c.String())
}