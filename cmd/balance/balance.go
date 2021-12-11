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
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/sboehler/knut/cmd/flags"
	"github.com/sboehler/knut/lib/balance"
	"github.com/sboehler/knut/lib/date"
	"github.com/sboehler/knut/lib/ledger"
	"github.com/sboehler/knut/lib/parser"
	"github.com/sboehler/knut/lib/report"
	"github.com/sboehler/knut/lib/table"

	"github.com/spf13/cobra"
)

// CreateCmd creates the command.
func CreateCmd() *cobra.Command {

	var r runner

	// Cmd is the balance command.
	var c = &cobra.Command{
		Use:   "balance",
		Short: "create a balance sheet",
		Long:  `Compute a balance for a date or set of dates.`,
		Args:  cobra.ExactValidArgs(1),
		Run:   r.run,
	}

	r.configureFlags(c)

	return c
}

type runner struct {
	cpuprofile                              string
	from, to                                flags.DateFlag
	last                                    int
	diff, showCommodities, thousands, color bool
	digits                                  int32
	accounts, commodities                   flags.RegexFlag
	period                                  flags.PeriodFlags
	mapping                                 flags.MappingFlag
	valuation                               flags.CommodityFlag
}

func (r *runner) run(cmd *cobra.Command, args []string) {
	if err := r.execute(cmd, args); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), err)
		os.Exit(1)
	}
}

func (r *runner) configureFlags(c *cobra.Command) {
	c.Flags().StringVar(&r.cpuprofile, "cpuprofile", "", "file to write profile")
	c.Flags().Var(&r.from, "from", "from date")
	c.Flags().Var(&r.to, "to", "to date")
	c.Flags().IntVar(&r.last, "last", 0, "last n periods")
	c.Flags().BoolVarP(&r.diff, "diff", "d", false, "diff")
	c.Flags().BoolVarP(&r.showCommodities, "show-commodities", "s", false, "Show commodities on their own rows")
	r.period.Setup(c.Flags())
	c.Flags().VarP(&r.valuation, "val", "v", "valuate in the given commodity")
	c.Flags().VarP(&r.mapping, "map", "m", "<level>,<regex>")
	c.Flags().Var(&r.accounts, "account", "filter accounts with a regex")
	c.Flags().Var(&r.commodities, "commodity", "filter commodities with a regex")
	c.Flags().Int32Var(&r.digits, "digits", 0, "round to number of digits")
	c.Flags().BoolVarP(&r.thousands, "thousands", "k", false, "show numbers in units of 1000")
	c.Flags().BoolVar(&r.color, "color", false, "print output in color")
}

func (r runner) execute(cmd *cobra.Command, args []string) error {
	if r.cpuprofile != "" {
		f, err := os.Create(r.cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	pipeline, err := r.configurePipeline(cmd, args)
	if err != nil {
		return err
	}
	var out = bufio.NewWriter(cmd.OutOrStdout())
	defer out.Flush()
	return processPipeline(out, pipeline)
}

type pipeline struct {
	Accounts        *ledger.Accounts
	Parser          parser.RecursiveParser
	Filter          ledger.Filter
	ProcessingSteps []ledger.Processor
	Balances        *[]*balance.Balance
	Report          *report.Report
	ReportRenderer  report.Renderer
	TextRenderer    table.TextRenderer
}

func (r runner) configurePipeline(cmd *cobra.Command, args []string) (*pipeline, error) {
	var (
		ctx = ledger.NewContext()
		err error
	)
	if time.Time(r.to).IsZero() {
		now := time.Now()
		r.to = flags.DateFlag(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
	}
	var (
		valuation *ledger.Commodity
		period    date.Period
	)
	if valuation, err = r.valuation.Value(ctx); err != nil {
		return nil, err
	}
	if period, err = r.period.Value(); err != nil {
		return nil, err
	}

	var (
		parser = parser.RecursiveParser{
			File:    args[0],
			Context: ctx,
		}
		bal      = balance.New(ctx, valuation)
		balances []*balance.Balance
		steps    = []ledger.Processor{
			balance.DateUpdater{Balance: bal},
			balance.AccountOpener{Balance: bal},
			balance.TransactionBooker{Balance: bal},
			balance.ValueBooker{Balance: bal},
			balance.Asserter{Balance: bal},
			&balance.PriceUpdater{Balance: bal},
			balance.TransactionValuator{Balance: bal},
			balance.ValuationTransactionComputer{Balance: bal},
			balance.AccountCloser{Balance: bal},
			&balance.Snapshotter{
				Balance: bal,
				From:    r.from.Value(),
				To:      r.to.Value(),
				Period:  period,
				Last:    r.last,
				Diff:    r.diff,
				Result:  &balances},
		}
		filter = ledger.Filter{
			Accounts:    r.accounts.Value(),
			Commodities: r.commodities.Value(),
		}
		rep = &report.Report{
			Value:   valuation != nil,
			Mapping: r.mapping.Value(),
		}
		reportRenderer = report.Renderer{
			Context:         ctx,
			ShowCommodities: r.showCommodities || valuation == nil,
			Report:          rep,
		}
		tableRenderer = table.TextRenderer{
			Color:     r.color,
			Thousands: r.thousands,
			Round:     r.digits,
		}
	)
	return &pipeline{
		Parser:          parser,
		Filter:          filter,
		ProcessingSteps: steps,
		Balances:        &balances,
		Report:          rep,
		ReportRenderer:  reportRenderer,
		TextRenderer:    tableRenderer,
	}, nil
}

func processPipeline(w io.Writer, ppl *pipeline) error {
	var (
		l   ledger.Ledger
		err error
	)
	if l, err = ppl.Parser.BuildLedger(ppl.Filter); err != nil {
		return err
	}
	if err = l.Process(ppl.ProcessingSteps); err != nil {
		return err
	}
	for _, bal := range *ppl.Balances {
		ppl.Report.Add(bal)
	}
	return ppl.TextRenderer.Render(ppl.ReportRenderer.Render(), w)
}
