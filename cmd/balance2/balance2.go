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

package balance2

import (
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/sboehler/knut/cmd/flags"
	"github.com/sboehler/knut/lib/common/amounts"
	"github.com/sboehler/knut/lib/common/cpr"
	"github.com/sboehler/knut/lib/common/date"
	"github.com/sboehler/knut/lib/journal"
	"github.com/sboehler/knut/lib/journal/ast"
	"github.com/sboehler/knut/lib/journal/process"

	"github.com/spf13/cobra"
)

// CreateCmd creates the command.
func CreateCmd() *cobra.Command {

	var r runner

	// Cmd is the balance command.
	var c = &cobra.Command{
		Use:   "balance2",
		Short: "create a balance sheet (experimental)",
		Long:  `Compute a balance for a date or set of dates.`,
		Args:  cobra.ExactValidArgs(1),
		Run:   r.run,
	}
	r.setupFlags(c)
	return c
}

type runner struct {
	cpuprofile                              string
	from, to                                flags.DateFlag
	last                                    int
	diff, showCommodities, thousands, color bool
	sortAlphabetically                      bool
	digits                                  int32
	accounts, commodities                   flags.RegexFlag
	interval                                flags.IntervalFlags
	mapping                                 flags.MappingFlag
	valuation                               flags.CommodityFlag
}

func (r *runner) run(cmd *cobra.Command, args []string) {
	if r.cpuprofile != "" {
		f, err := os.Create(r.cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if err := r.execute(cmd, args); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), err)
		os.Exit(1)
	}
}

func (r *runner) setupFlags(c *cobra.Command) {
	c.Flags().StringVar(&r.cpuprofile, "cpuprofile", "", "file to write profile")
	c.Flags().Var(&r.from, "from", "from date")
	c.Flags().Var(&r.to, "to", "to date")
	c.Flags().IntVar(&r.last, "last", 0, "last n periods")
	c.Flags().BoolVarP(&r.diff, "diff", "d", false, "diff")
	c.Flags().BoolVarP(&r.sortAlphabetically, "sort", "a", false, "Sort accounts alphabetically")
	c.Flags().BoolVarP(&r.showCommodities, "show-commodities", "s", false, "Show commodities on their own rows")
	r.interval.Setup(c.Flags())
	c.Flags().VarP(&r.valuation, "val", "v", "valuate in the given commodity")
	c.Flags().VarP(&r.mapping, "map", "m", "<level>,<regex>")
	c.Flags().Var(&r.accounts, "account", "filter accounts with a regex")
	c.Flags().Var(&r.commodities, "commodity", "filter commodities with a regex")
	c.Flags().Int32Var(&r.digits, "digits", 0, "round to number of digits")
	c.Flags().BoolVarP(&r.thousands, "thousands", "k", false, "show numbers in units of 1000")
	c.Flags().BoolVar(&r.color, "color", false, "print output in color")
}

func (r runner) execute(cmd *cobra.Command, args []string) error {
	var (
		ctx       = cmd.Context()
		jctx      = journal.NewContext()
		valuation *journal.Commodity
		interval  date.Interval
		err       error
	)
	if time.Time(r.to).IsZero() {
		r.to = flags.DateFlag(date.Today())
	}
	if valuation, err = r.valuation.Value(jctx); err != nil {
		return err
	}
	if interval, err = r.interval.Value(); err != nil {
		return err
	}
	journalSource := &process.JournalSource{
		Context: jctx,
		Path:    args[0],
		Filter: journal.Filter{
			Accounts:    r.accounts.Value(),
			Commodities: r.commodities.Value(),
		},
		Expand: true,
	}
	if err := journalSource.Load(ctx); err != nil {
		return err
	}
	var (
		priceUpdater = &process.PriceUpdater{
			Context:   jctx,
			Valuation: valuation,
		}
		balancer = &process.Balancer{
			Context: jctx,
		}
		valuator = &process.Valuator{
			Context:   jctx,
			Valuation: valuation,
		}
		aggregator = &process.Aggregator{
			Context:   jctx,
			Valuation: valuation,
			Mappers: amounts.Combine(
				amounts.Account{
					Context: jctx,
					Mapping: r.mapping.Value(),
				}.Mapper(),
				amounts.Commodity{
					Show: r.showCommodities,
				}.Mapper(),
				amounts.TimePartition{
					From:     r.from.ValueOr(journalSource.Min()),
					To:       r.to.ValueOr(date.Today()),
					Interval: interval,
					Last:     r.last}.Mapper(),
			),
		}
	)

	s := cpr.Compose[*ast.Day, *ast.Day](journalSource, priceUpdater)
	s = cpr.Compose[*ast.Day, *ast.Day](s, balancer)
	s = cpr.Compose[*ast.Day, *ast.Day](s, valuator)
	ppl := cpr.Connect[*ast.Day](s, aggregator)

	if err := ppl.Process(ctx); err != nil {
		return err
	}
	return nil
	// out := bufio.NewWriter(cmd.OutOrStdout())
	// defer out.Flush()
	// return tableRenderer.Render(reportRenderer.Render(reportBuilder.Result), out)
}