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

package commands

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/pprof"

	"github.com/sboehler/knut/cmd/flags"
	"github.com/sboehler/knut/lib/amounts"
	"github.com/sboehler/knut/lib/common/mapper"
	"github.com/sboehler/knut/lib/common/predicate"
	"github.com/sboehler/knut/lib/common/table"
	"github.com/sboehler/knut/lib/journal"
	"github.com/sboehler/knut/lib/journal/check"
	"github.com/sboehler/knut/lib/model"
	"github.com/sboehler/knut/lib/model/account"
	"github.com/sboehler/knut/lib/model/commodity"
	"github.com/sboehler/knut/lib/model/registry"
	"github.com/sboehler/knut/lib/reports/balance"

	"github.com/spf13/cobra"
)

// CreateBalanceCommand creates the command.
func CreateBalanceCommand() *cobra.Command {

	var r balanceRunner

	// Cmd is the balance command.
	c := &cobra.Command{
		Use:   "balance",
		Short: "create a balance sheet",
		Long:  `Compute a balance for a date or set of dates.`,
		Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Run:   r.run,
	}
	r.setupFlags(c)
	return c
}

type balanceRunner struct {
	flags.Multiperiod

	// internal
	cpuprofile string

	// journal structure
	close     bool
	valuation flags.CommodityFlag

	// mapping
	mapping flags.MappingFlag
	remap   flags.RegexFlag

	// filters
	accounts    flags.RegexFlag
	commodities flags.RegexFlag

	// report structure
	diff               bool
	showCommodities    flags.RegexFlag
	sortAlphabetically bool

	// formatting
	thousands bool
	color     bool
	digits    int32
	csv       bool
}

func (r *balanceRunner) run(cmd *cobra.Command, args []string) {
	if r.cpuprofile != "" {
		f, err := os.Create(r.cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if err := r.execute(cmd, args); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%+v\n", err)
		os.Exit(1)
	}
}

func (r *balanceRunner) setupFlags(c *cobra.Command) {
	r.Multiperiod.Setup(c)
	c.Flags().StringVar(&r.cpuprofile, "cpuprofile", "", "file to write profile")
	c.Flags().BoolVarP(&r.diff, "diff", "d", false, "diff")
	c.Flags().BoolVarP(&r.csv, "csv", "", false, "csv")
	c.Flags().BoolVar(&r.close, "close", true, "close")
	c.Flags().BoolVarP(&r.sortAlphabetically, "sort", "a", false, "Sort accounts alphabetically")
	c.Flags().VarP(&r.showCommodities, "show-commodities", "s", "<regex>")
	c.Flags().VarP(&r.valuation, "val", "v", "valuate in the given commodity")
	c.Flags().VarP(&r.mapping, "map", "m", "<level>,<regex>")
	c.Flags().VarP(&r.remap, "remap", "r", "<regex>")
	c.Flags().Var(&r.accounts, "account", "filter accounts with a regex")
	c.Flags().Var(&r.commodities, "commodity", "filter commodities with a regex")
	c.Flags().Int32Var(&r.digits, "digits", 0, "round to number of digits")
	c.Flags().BoolVarP(&r.thousands, "thousands", "k", false, "show numbers in units of 1000")
	c.Flags().BoolVar(&r.color, "color", true, "print output in color")
}

func (r balanceRunner) execute(cmd *cobra.Command, args []string) error {
	reg := registry.New()
	valuation, err := r.valuation.Value(reg)
	if err != nil {
		return err
	}
	j, err := journal.FromPath(cmd.Context(), reg, args[0])
	if err != nil {
		return err
	}
	partition := r.Multiperiod.Partition(j.Period())
	report := balance.NewReport(reg, partition)
	procs := []*journal.Processor{
		check.Check(),
		journal.ComputePrices(valuation),
		journal.Valuate(reg, valuation),
		journal.Filter(partition),
		journal.CloseAccounts(j, reg, r.close, partition),
		journal.Query{
			Select: amounts.KeyMapper{
				Date: partition.Align(),
				Account: mapper.Sequence(
					account.Remap(reg.Accounts(), r.remap.Regex()),
					account.Shorten(reg.Accounts(), r.mapping.Value()),
				),
				Commodity: mapper.Identity[*model.Commodity],
				Valuation: commodity.IdentityIf(valuation != nil),
			}.Build(),
			Where: predicate.And(
				amounts.AccountMatches(r.accounts.Regex()),
				amounts.CommodityMatches(r.commodities.Regex()),
			),
			Valuation: valuation,
		}.Into(report),
	}
	err = j.Build().Process(procs...)
	if err != nil {
		return err
	}
	reportRenderer := balance.Renderer{
		Valuation:          valuation,
		CommodityDetails:   r.showCommodities.Regex(),
		SortAlphabetically: r.sortAlphabetically,
		Diff:               r.diff,
	}
	var tableRenderer Renderer
	if r.csv {
		tableRenderer = &table.CSVRenderer{}
	} else {
		tableRenderer = &table.TextRenderer{
			Color:     r.color,
			Thousands: r.thousands,
			Round:     r.digits,
		}
	}
	out := bufio.NewWriter(cmd.OutOrStdout())
	defer out.Flush()
	return tableRenderer.Render(reportRenderer.Render(report), out)
}

type Renderer interface {
	Render(*table.Table, io.Writer) error
}
