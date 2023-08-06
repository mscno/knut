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

package journal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sboehler/knut/lib/common/compare"
	"github.com/sboehler/knut/lib/common/cpr"
	"github.com/sboehler/knut/lib/common/date"
	"github.com/sboehler/knut/lib/common/dict"
	"github.com/sboehler/knut/lib/common/slice"
	"github.com/sboehler/knut/lib/model"
	"github.com/sboehler/knut/lib/model/price"
	"github.com/sboehler/knut/lib/syntax/parser"
	"github.com/sourcegraph/conc/pool"
)

// Journal represents an unprocessed
type Journal struct {
	Registry *model.Registry
	Days     map[time.Time]*Day
	min, max time.Time
}

// New creates a new Journal.
func New(reg *model.Registry) *Journal {
	return &Journal{
		Registry: reg,
		Days:     make(map[time.Time]*Day),
		min:      date.Date(9999, 12, 31),
		max:      time.Time{},
	}
}

// Day returns the Day for the given date.
func (j *Journal) Day(d time.Time) *Day {
	return dict.GetDefault(j.Days, d, func() *Day { return &Day{Date: d} })
}

func (j *Journal) Sorted() []*Day {
	l, _ := j.Process(Sort())
	return l
}

// AddOpen adds an Open directive.
func (j *Journal) AddOpen(o *model.Open) {
	d := j.Day(o.Date)
	d.Openings = append(d.Openings, o)
}

// AddPrice adds an Price directive.
func (j *Journal) AddPrice(p *model.Price) {
	d := j.Day(p.Date)
	if j.max.Before(d.Date) {
		j.max = d.Date
	}
	d.Prices = append(d.Prices, p)
}

// AddTransaction adds an Transaction directive.
func (j *Journal) AddTransaction(t *model.Transaction) {
	d := j.Day(t.Date)
	if j.max.Before(d.Date) {
		j.max = d.Date
	}
	if j.min.After(t.Date) {
		j.min = d.Date
	}
	d.Transactions = append(d.Transactions, t)
}

// AddAssertion adds an Assertion directive.
func (j *Journal) AddAssertion(a *model.Assertion) {
	d := j.Day(a.Date)
	d.Assertions = append(d.Assertions, a)
}

// AddClose adds an Close directive.
func (j *Journal) AddClose(c *model.Close) {
	d := j.Day(c.Date)
	d.Closings = append(d.Closings, c)
}

func (j *Journal) Period() date.Period {
	return date.Period{Start: j.min, End: j.max}
}

func (j *Journal) Process(fs ...func(*Day) error) ([]*Day, error) {
	ds := dict.SortedValues(j.Days, CompareDays)
	ds, err := slice.Parallel(ds, fs...)
	if err != nil {
		return nil, err
	}
	return ds, nil
}

func FromPath(ctx context.Context, reg *model.Registry, path string) (*Journal, error) {
	syntaxCh, worker1 := parser.Parse(path)
	modelCh, worker2 := model.FromStream(reg, syntaxCh)
	journalCh, worker3 := Create(reg, modelCh)
	p := pool.New().WithErrors().WithFirstError().WithContext(ctx)
	p.Go(worker1)
	p.Go(worker2)
	p.Go(worker3)
	err := p.Wait()
	if err != nil {
		return nil, err
	}
	return <-journalCh, nil
}

func Create(reg *model.Registry, modelCh <-chan []any) (<-chan *Journal, func(context.Context) error) {
	return cpr.FanIn(func(ctx context.Context, ch chan<- *Journal) error {
		j := New(reg)
		err := cpr.Consume(ctx, modelCh, func(input []any) error {
			for _, d := range input {
				switch t := d.(type) {
				case *model.Price:
					j.AddPrice(t)

				case *model.Open:
					j.AddOpen(t)

				case *model.Transaction:
					j.AddTransaction(t)

				case *model.Assertion:
					j.AddAssertion(t)

				case *model.Close:
					j.AddClose(t)

				default:
					return fmt.Errorf("unknown: %v (%T)", t, t)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		return cpr.Push(ctx, ch, j)
	})
}

// Day groups all commands for a given date.
type Day struct {
	Date         time.Time
	Prices       []*model.Price
	Assertions   []*model.Assertion
	Openings     []*model.Open
	Transactions []*model.Transaction
	Closings     []*model.Close

	Normalized price.NormalizedPrices

	Performance *Performance
}

// Less establishes an ordering on Day.
func CompareDays(d *Day, d2 *Day) compare.Order {
	return compare.Time(d.Date, d2.Date)
}

// Performance holds aggregate information used to compute
// portfolio performance.
type Performance struct {
	V0, V1, Inflow, Outflow, InternalInflow, InternalOutflow map[*model.Commodity]float64
	PortfolioInflow, PortfolioOutflow                        float64
}

func (p Performance) String() string {
	var buf strings.Builder
	for c, v := range p.V0 {
		fmt.Fprintf(&buf, "V0: %20s %f\n", c, v)
	}
	for c, f := range p.Inflow {
		fmt.Fprintf(&buf, "Inflow: %20s %f\n", c, f)
	}
	for c, f := range p.Outflow {
		fmt.Fprintf(&buf, "Outflow: %20s %f\n", c, f)
	}
	for c, f := range p.InternalInflow {
		fmt.Fprintf(&buf, "InternalInflow: %20s %f\n", c, f)
	}
	for c, f := range p.InternalOutflow {
		fmt.Fprintf(&buf, "InternalOutflow: %20s %f\n", c, f)
	}
	for c, v := range p.V1 {
		fmt.Fprintf(&buf, "V1: %20s %f\n", c, v)
	}
	return buf.String()
}