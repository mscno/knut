// Copyright 2021 Silvio Böhler
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package balance

import (
	"fmt"
	"strings"
	"time"

	"github.com/sboehler/knut/lib/balance/prices"
	"github.com/sboehler/knut/lib/journal"
	"github.com/sboehler/knut/lib/journal/ast"
	"github.com/sboehler/knut/lib/journal/ast/printer"

	"github.com/shopspring/decimal"
)

// Balance represents a balance for accounts at the given date.
type Balance struct {
	Date             time.Time
	Amounts, Values  map[CommodityAccount]decimal.Decimal
	Accounts         Accounts
	Context          journal.Context
	Valuation        *journal.Commodity
	NormalizedPrices prices.NormalizedPrices
}

// New creates a new balance.
func New(ctx journal.Context, valuation *journal.Commodity) *Balance {
	return &Balance{
		Context:   ctx,
		Amounts:   make(map[CommodityAccount]decimal.Decimal),
		Values:    make(map[CommodityAccount]decimal.Decimal),
		Accounts:  make(Accounts),
		Valuation: valuation,
	}
}

// Snapshot deeply copies the balance
func (b *Balance) Snapshot() *Balance {
	var nb = New(b.Context, b.Valuation)
	nb.Date = b.Date
	nb.NormalizedPrices = b.NormalizedPrices
	for pos, amt := range b.Amounts {
		nb.Amounts[pos] = amt
	}
	for pos, val := range b.Values {
		nb.Values[pos] = val
	}
	nb.Accounts = b.Accounts.Copy()
	return nb
}

// Minus mutably subtracts the given balance from the receiver.
func (b *Balance) Minus(bo *Balance) {
	for pos, va := range bo.Amounts {
		b.Amounts[pos] = b.Amounts[pos].Sub(va)
	}
	for pos, va := range bo.Values {
		b.Values[pos] = b.Values[pos].Sub(va)
	}
}

// BookAmount books the given amount.
func (b *Balance) BookAmount(t *ast.Transaction) error {
	for _, posting := range t.Postings {
		if !b.Accounts.IsOpen(posting.Credit) {
			return Error{t, fmt.Sprintf("credit account %s is not open", posting.Credit)}
		}
		if !b.Accounts.IsOpen(posting.Debit) {
			return Error{t, fmt.Sprintf("debit account %s is not open", posting.Debit)}
		}
		b.Book(posting.Credit, posting.Debit, posting.Amount, posting.Commodity)
	}
	return nil
}

// Amount returns the amount for the given account and commodity.
func (b *Balance) Amount(a *journal.Account, c *journal.Commodity) decimal.Decimal {
	return b.Amounts[CommodityAccount{Account: a, Commodity: c}]
}

// Book books the given amount.
func (b *Balance) Book(cr, dr *journal.Account, a decimal.Decimal, c *journal.Commodity) {
	var (
		crPos = CommodityAccount{cr, c}
		drPos = CommodityAccount{dr, c}
	)
	b.Amounts[crPos] = b.Amounts[crPos].Sub(a)
	b.Amounts[drPos] = b.Amounts[drPos].Add(a)
}

func (b *Balance) bookValue(t *ast.Transaction) error {
	for _, posting := range t.Postings {
		var (
			crPos = CommodityAccount{posting.Credit, posting.Commodity}
			drPos = CommodityAccount{posting.Debit, posting.Commodity}
		)
		b.Values[crPos] = b.Values[crPos].Sub(posting.Value)
		b.Values[drPos] = b.Values[drPos].Add(posting.Value)
	}
	return nil
}

// Diffs creates the difference balances for the given
// slice of balances. The returned slice is one element smaller
// than the input slice. The balances are mutated.
func Diffs(bals []*Balance) []*Balance {
	for i := len(bals) - 1; i > 0; i-- {
		bals[i].Minus(bals[i-1])
	}
	return bals[1:]
}

// Error is an error.
type Error struct {
	directive ast.Directive
	msg       string
}

func (be Error) Error() string {
	var (
		p printer.Printer
		b strings.Builder
	)
	fmt.Fprintf(&b, "%s:\n", be.directive.Position().Start)
	p.PrintDirective(&b, be.directive)
	fmt.Fprintf(&b, "\n%s\n", be.msg)
	return b.String()
}

// CommodityAccount represents a position.
type CommodityAccount struct {
	Account   *journal.Account
	Commodity *journal.Commodity
}

// Less establishes a partial ordering of commodity accounts.
func (p CommodityAccount) Less(p1 CommodityAccount) bool {
	if p.Account.Type() != p1.Account.Type() {
		return p.Account.Type() < p1.Account.Type()
	}
	if p.Account.String() != p1.Account.String() {
		return p.Account.String() < p1.Account.String()
	}
	return p.Commodity.String() < p1.Commodity.String()
}
