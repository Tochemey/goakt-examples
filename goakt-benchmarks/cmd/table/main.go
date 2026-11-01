// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// Command table renders saved benchmark results as Markdown comparison tables.
//
// Each argument is label=path, where path is the output of go test -bench for
// one framework. The first label is the baseline the other columns are compared
// with. -match and -skip keep or drop benchmarks whose name matches a regular
// expression:
//
//	go run ./cmd/table -skip '^Grain' goakt=results/goakt.txt ergo=results/ergo.txt
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/perf/benchfmt"
)

// metric is rendered as its own table. A benchmark row uses the first of the
// metric's units it reports. A benchmark that reports an exclusive metric's
// unit appears in that table only: its other figures describe the setup, not
// the quantity measured.
type metric struct {
	units        []string
	title        string
	higherBetter bool
	exclusive    bool
	format       func(value float64, unit string) string
}

var metrics = []metric{
	{units: []string{"msgs/s", "actors/s", "grains/s"}, title: "Throughput", higherBetter: true, format: formatRate},
	{units: []string{"ns/op"}, title: "Time per operation", format: formatDuration},
	{units: []string{"B/op"}, title: "Bytes allocated per operation", format: func(v float64, _ string) string { return fmt.Sprintf("%.0f B", v) }},
	{units: []string{"allocs/op"}, title: "Allocations per operation", format: func(v float64, _ string) string { return fmt.Sprintf("%.0f", v) }},
	{units: []string{"B/actor"}, title: "Live heap per idle actor", exclusive: true, format: func(v float64, _ string) string { return fmt.Sprintf("%.0f B", v) }},
	{units: []string{"B/grain"}, title: "Live heap per idle grain", exclusive: true, format: func(v float64, _ string) string { return fmt.Sprintf("%.0f B", v) }},
}

// samples holds every value recorded for a benchmark, keyed by framework label
// and then by unit.
type samples map[string]map[string][]float64

func main() {
	match := flag.String("match", "", "only render benchmarks whose name matches this regular expression")
	skip := flag.String("skip", "", "leave out benchmarks whose name matches this regular expression")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: table [-match regexp] [-skip regexp] label=path [label=path...]")
		os.Exit(2)
	}
	keep, err := filter(*match, *skip)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	var (
		labels     []string
		benchmarks []string
		results    = make(map[string]samples)
		cpu        string
	)

	for _, arg := range flag.Args() {
		label, path, ok := strings.Cut(arg, "=")
		if !ok {
			fmt.Fprintf(os.Stderr, "invalid argument %q: want label=path\n", arg)
			os.Exit(2)
		}
		labels = append(labels, label)

		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		reader := benchfmt.NewReader(file, path)
		for reader.Scan() {
			// Lines that are not results, such as a failed run, are skipped.
			result, ok := reader.Result().(*benchfmt.Result)
			if !ok {
				continue
			}
			if cpu == "" {
				cpu = result.GetConfig("cpu")
			}

			name := result.Name.String()
			if !keep(name) {
				continue
			}
			if _, ok := results[name]; !ok {
				results[name] = make(samples)
				benchmarks = append(benchmarks, name)
			}
			if _, ok := results[name][label]; !ok {
				results[name][label] = make(map[string][]float64)
			}
			for _, value := range result.Values {
				// ns/op is tidied into sec/op on read; the other units are kept as is.
				unit, v := value.Unit, value.Value
				if value.OrigUnit != "" {
					unit, v = value.OrigUnit, value.OrigValue
				}
				results[name][label][unit] = append(results[name][label][unit], v)
			}
		}
		_ = file.Close()
		if err := reader.Err(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	if cpu != "" {
		fmt.Printf("Measured on %s. Each cell is the median of all runs, the best value in a row is in bold, and the factor in parentheses is relative to %s.\n\n", cpu, labels[0])
	}

	for _, m := range metrics {
		var rows []string
		for _, name := range benchmarks {
			if m.exclusive {
				if !slices.ContainsFunc(m.units, func(unit string) bool { return hasUnit(results[name], unit) }) {
					continue
				}
			} else if hasExclusiveUnit(results[name]) {
				continue
			}

			unit := m.units[0]
			for _, candidate := range m.units {
				if hasUnit(results[name], candidate) {
					unit = candidate
					break
				}
			}

			medians := make([]float64, len(labels))
			found := make([]bool, len(labels))
			for i, label := range labels {
				if values := results[name][label][unit]; len(values) > 0 {
					medians[i], found[i] = median(values), true
				}
			}

			best, hasBest := 0.0, false
			for i := range labels {
				if !found[i] {
					continue
				}
				if !hasBest || (m.higherBetter && medians[i] > best) || (!m.higherBetter && medians[i] < best) {
					best, hasBest = medians[i], true
				}
			}

			cells := make([]string, len(labels))
			for i := range labels {
				if !found[i] {
					cells[i] = "n/a"
					continue
				}
				cell := m.format(medians[i], unit)
				if i > 0 && found[0] && medians[0] > 0 {
					cell += fmt.Sprintf(" (%.2f×)", medians[i]/medians[0])
				}
				if medians[i] == best {
					cell = "**" + cell + "**"
				}
				cells[i] = cell
			}
			rows = append(rows, fmt.Sprintf("| %s | %s |\n", name, strings.Join(cells, " | ")))
		}
		if len(rows) == 0 {
			continue
		}

		fmt.Printf("### %s\n\n", m.title)
		if m.higherBetter {
			fmt.Print("Higher is better.\n\n")
		} else {
			fmt.Print("Lower is better.\n\n")
		}
		fmt.Printf("| Benchmark | %s |\n", strings.Join(labels, " | "))
		fmt.Printf("|---|%s\n", strings.Repeat("---:|", len(labels)))
		fmt.Print(strings.Join(rows, ""))
		fmt.Println()
	}
}

// filter returns a function reporting whether a benchmark is rendered: its name
// matches match, when set, and does not match skip, when set.
func filter(match, skip string) (func(name string) bool, error) {
	var matchRe, skipRe *regexp.Regexp
	var err error
	if match != "" {
		if matchRe, err = regexp.Compile(match); err != nil {
			return nil, fmt.Errorf("invalid -match: %w", err)
		}
	}
	if skip != "" {
		if skipRe, err = regexp.Compile(skip); err != nil {
			return nil, fmt.Errorf("invalid -skip: %w", err)
		}
	}
	return func(name string) bool {
		return (matchRe == nil || matchRe.MatchString(name)) && (skipRe == nil || !skipRe.MatchString(name))
	}, nil
}

// hasExclusiveUnit reports whether a benchmark recorded the unit of an
// exclusive metric.
func hasExclusiveUnit(byLabel samples) bool {
	for _, m := range metrics {
		if !m.exclusive {
			continue
		}
		for _, unit := range m.units {
			if hasUnit(byLabel, unit) {
				return true
			}
		}
	}
	return false
}

// hasUnit reports whether any framework recorded unit for a benchmark.
func hasUnit(byLabel samples, unit string) bool {
	for _, units := range byLabel {
		if len(units[unit]) > 0 {
			return true
		}
	}
	return false
}

func median(values []float64) float64 {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func formatDuration(ns float64, _ string) string {
	switch {
	case ns >= 1e6:
		return fmt.Sprintf("%.2f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.2f µs", ns/1e3)
	default:
		return fmt.Sprintf("%.1f ns", ns)
	}
}

func formatRate(rate float64, unit string) string {
	switch {
	case rate >= 1e6:
		return fmt.Sprintf("%.2fM %s", rate/1e6, unit)
	case rate >= 1e3:
		return fmt.Sprintf("%.0fK %s", rate/1e3, unit)
	default:
		return fmt.Sprintf("%.0f %s", rate, unit)
	}
}
