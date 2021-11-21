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

package ledger

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/sboehler/knut/lib/date"
	"github.com/sboehler/knut/lib/scanner"
)

// Day groups all commands for a given date.
type Day struct {
	Date         time.Time
	Prices       []Price
	Assertions   []Assertion
	Values       []Value
	Openings     []Open
	Transactions []Transaction
	Closings     []Close
}

// Ledger is a
type Ledger struct {
	Days    []*Day
	Context Context
}

// MinDate returns the minimum date for this ledger, as the first
// date on which an account is opened (ignoring prices, for example).
func (l Ledger) MinDate() (time.Time, bool) {
	for _, s := range l.Days {
		if len(s.Openings) > 0 {
			return s.Date, true
		}
	}
	return time.Time{}, false
}

// MaxDate returns the maximum date for the given
func (l Ledger) MaxDate() (time.Time, bool) {
	if len(l.Days) == 0 {
		return time.Time{}, false
	}
	return l.Days[len(l.Days)-1].Date, true
}

// Dates returns a series of dates.
func (l Ledger) Dates(from, to *time.Time, period date.Period) []time.Time {
	var t0, t1 time.Time
	if from != nil {
		t0 = *from
	} else if d, ok := l.MinDate(); ok {
		t0 = d
	} else {
		return nil
	}
	if to != nil {
		t1 = *to
	} else if d, ok := l.MaxDate(); ok {
		t1 = d
	} else {
		return nil
	}
	return date.Series(t0, t1, period)
}

// Range describes a range of locations in a file.
type Range struct {
	Path       string
	Start, End scanner.Location
}

// Position returns the Range itself.
func (r Range) Position() Range {
	return r
}

// Directive is an element in a journal with a position.
type Directive interface {
	Position() Range
}

var (
	_ Directive = (*Open)(nil)
	_ Directive = (*Close)(nil)
	_ Directive = (*Transaction)(nil)
	_ Directive = (*Value)(nil)
	_ Directive = (*Assertion)(nil)
	_ Directive = (*Price)(nil)
	_ Directive = (*Include)(nil)
	_ Directive = (*Accrual)(nil)
)

// Open represents an open command.
type Open struct {
	Range
	Date    time.Time
	Account *Account
}

// Close represents a close command.
type Close struct {
	Range
	Date    time.Time
	Account *Account
}

// Posting represents a posting.
type Posting struct {
	Amount, Value decimal.Decimal
	Credit, Debit *Account
	Commodity     *Commodity
	Lot           *Lot
}

// NewPosting creates a new posting from the given parameters. If amount is negative, it
// will be inverted and the accounts reversed.
func NewPosting(crAccount, drAccount *Account, commodity *Commodity, amt decimal.Decimal) Posting {
	if amt.IsNegative() {
		crAccount, drAccount = drAccount, crAccount
		amt = amt.Neg()
	}
	return Posting{
		Credit:    crAccount,
		Debit:     drAccount,
		Amount:    amt,
		Commodity: commodity,
	}
}

// Lot represents a lot.
type Lot struct {
	Date      time.Time
	Label     string
	Price     float64
	Commodity *Commodity
}

// Tag represents a tag for a transaction or booking.
type Tag string

// Transaction represents a transaction.
type Transaction struct {
	Range
	Date        time.Time
	Description string
	Tags        []Tag
	Postings    []Posting
}

// Price represents a price command.
type Price struct {
	Range
	Date      time.Time
	Commodity *Commodity
	Target    *Commodity
	Price     decimal.Decimal
}

// Include represents an include directive.
type Include struct {
	Range
	Date time.Time
	Path string
}

// Assertion represents a balance assertion.
type Assertion struct {
	Range
	Date      time.Time
	Account   *Account
	Amount    decimal.Decimal
	Commodity *Commodity
}

// Value represents a value directive.
type Value struct {
	Range
	Date      time.Time
	Account   *Account
	Amount    decimal.Decimal
	Commodity *Commodity
}

// Accrual represents an accrual.
type Accrual struct {
	Range
	Period      date.Period
	T0, T1      time.Time
	Account     *Account
	Transaction Transaction
}

// Expand expands an accrual transaction.
func (a Accrual) Expand() []Transaction {
	var (
		t                                                                = a.Transaction
		posting                                                          = t.Postings[0]
		crAccountSingle, drAccountSingle, crAccountMulti, drAccountMulti = a.Account, a.Account, a.Account, a.Account
	)
	switch {
	case isAL(posting.Credit) && isIE(posting.Debit):
		crAccountSingle = posting.Credit
		drAccountMulti = posting.Debit
	case isIE(posting.Credit) && isAL(posting.Debit):
		crAccountMulti = posting.Credit
		drAccountSingle = posting.Debit
	case isIE(posting.Credit) && isIE(posting.Debit):
		crAccountMulti = posting.Credit
		drAccountMulti = posting.Debit
	default:
		crAccountSingle = posting.Credit
		drAccountSingle = posting.Debit
	}
	var (
		dates       = date.Series(a.T0, a.T1, a.Period)[1:]
		amount, rem = posting.Amount.QuoRem(decimal.NewFromInt(int64(len(dates))), 1)

		result []Transaction
	)
	if crAccountMulti != drAccountMulti {
		for i, date := range dates {
			var a = amount
			if i == 0 {
				a = a.Add(rem)
			}
			result = append(result, Transaction{
				Range:       t.Range,
				Date:        date,
				Tags:        t.Tags,
				Description: fmt.Sprintf("%s (accrual %d/%d)", t.Description, i+1, len(dates)),
				Postings: []Posting{
					NewPosting(crAccountMulti, drAccountMulti, posting.Commodity, a),
				},
			})
		}
	}
	if crAccountSingle != drAccountSingle {
		result = append(result, Transaction{
			Range:       t.Range,
			Date:        t.Date,
			Tags:        t.Tags,
			Description: t.Description,
			Postings: []Posting{
				NewPosting(crAccountSingle, drAccountSingle, posting.Commodity, posting.Amount),
			},
		})

	}
	return result
}

func isAL(a *Account) bool {
	return a.Type() == ASSETS || a.Type() == LIABILITIES
}

func isIE(a *Account) bool {
	return a.Type() == INCOME || a.Type() == EXPENSES
}
