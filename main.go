package main

import (
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/cosmo-workspace/controller-testtools/pkg/charts"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
)

var (
	// goreleaser default https://goreleaser.com/customization/builds/
	version = "snapshot"
	commit  = "snapshot"
	date    = "snapshot"
	o       = &option{}
	log     *slog.Logger
	values  []string
)

type option struct {
	HelmPath       string
	ReleaseName    string
	Namespace      string
	Chart          string
	ValuesFile     string
	Debug          bool
	UpdateSnapshot bool
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "chartsnap",
		Short: "Snapshot testing tool for Helm charts",
		Long: `
Snapshot testing tool like Jest for Helm charts.

You can create test cases as a variation of Values files of your chart.
` + "`" + `__snapshot__` + "`" + ` directory is created in the same directory as test Values files.
In addition, Values files can have a ` + "`" + `testSpec` + "`" + ` property that can detail or control the test case.

` + "```" + `yaml
testSpec:
  # desc is a description for the set of values
  desc: only required values and the rest is default
  # dynamicFields defines values that are dynamically generated by Helm function like 'randAlphaNum'
  # https://helm.sh/docs/chart_template_guide/function_list/#randalphanum-randalpha-randnumeric-and-randascii
  # Replace outputs with fixed values to prevent unmatched outputs at each snapshot.
  dynamicFields:
    - apiVersion: v1
      kind: Secret
      name: cosmo-auth-env
      jsonPath:
        - /data/COOKIE_HASHKEY
        - /data/COOKIE_BLOCKKEY
        - /data/COOKIE_HASHKEY
        - /data/COOKIE_SESSION_NAME

# Others can be any your chart value.
# ...
` + "```" + `

See the repository for full documentation.
https://github.com/jlandowner/helm-chartsnap.git

MIT 2023 jlandowner/helm-chartsnap
`,
		Example: `
  # Snapshot with defualt values:
  chartsnap -c YOUR_CHART
  
  # Update snapshot files:
  chartsnap -c YOUR_CHART -u

  # Snapshot with test case values:
  chartsnap -c YOUR_CHART -f YOUR_TEST_VALUES_FILE
  
  # Snapshot all test cases:
  chartsnap -c YOUR_CHART -f YOUR_TEST_VALUES_FILES_DIRECTOY
  
  # Set addtional args or flags for 'helm template' command:
  chartsnap -c YOUR_CHART -f YOUR_TEST_VALUES_FILE -- --skip-tests`,
		Version: fmt.Sprintf("version=%s commit=%s date=%s", version, commit, date),
		RunE:    run,
	}
	rootCmd.PersistentFlags().BoolVar(&o.Debug, "debug", false, "debug mode")
	rootCmd.PersistentFlags().BoolVarP(&o.UpdateSnapshot, "update-snapshot", "u", false, "update snapshot mode")
	rootCmd.PersistentFlags().StringVarP(&o.Chart, "chart", "c", "", "path to the chart directory. this flag is passed to 'helm template RELEASE_NAME CHART --values VALUES' as 'CHART'")
	if err := rootCmd.MarkPersistentFlagDirname("chart"); err != nil {
		panic(err)
	}
	if err := rootCmd.MarkPersistentFlagRequired("chart"); err != nil {
		panic(err)
	}
	rootCmd.PersistentFlags().StringVar(&o.ReleaseName, "release-name", "testrelease", "release name. this flag is passed to 'helm template RELEASE_NAME CHART --values VALUES' as 'RELEASE_NAME'")
	rootCmd.PersistentFlags().StringVar(&o.Namespace, "namespace", "testns", "namespace. this flag is passed to 'helm template RELEASE_NAME CHART --values VALUES --namespace NAMESPACE' as 'NAMESPACE'")
	rootCmd.PersistentFlags().StringVar(&o.HelmPath, "helm-path", "helm", "path to the helm command")
	rootCmd.PersistentFlags().StringVarP(&o.ValuesFile, "values", "f", "", "path to a test values file or directory. if directroy is set, all test files are tested. if empty, default values are used. this flag is passed to 'helm template RELEASE_NAME CHART --values VALUES' as 'VALUES'")

	if err := rootCmd.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: func() slog.Leveler {
			if o.Debug {
				return slog.LevelDebug
			}
			return slog.LevelInfo
		}(),
	}))
	log.Debug("options", printOptions(*o)...)

	if o.ValuesFile == "" {
		values = []string{""}
	} else {
		if s, err := os.Stat(o.ValuesFile); os.IsNotExist(err) {
			return fmt.Errorf("values file '%s' not found", o.ValuesFile)
		} else if s.IsDir() {
			// get all values files in the directory
			files, err := os.ReadDir(o.ValuesFile)
			if err != nil {
				return fmt.Errorf("failed to read values file directory: %w", err)
			}
			values = make([]string, 0)
			for _, f := range files {
				// read only *.yaml
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".yaml") {
					values = append(values, path.Join(o.ValuesFile, f.Name()))
				}
			}
		} else {
			values = []string{o.ValuesFile}
		}
	}

	eg, ctx := errgroup.WithContext(cmd.Context())
	for _, v := range values {
		ht := charts.HelmTemplateCmdOptions{
			HelmPath:       o.HelmPath,
			ReleaseName:    o.ReleaseName,
			Namespace:      o.Namespace,
			Chart:          o.Chart,
			ValuesFile:     v,
			AdditionalArgs: args,
		}
		bannerPrintln("RUNS",
			fmt.Sprintf("Snapshot testing chart=%s values=%s", ht.Chart, ht.ValuesFile), 0, color.BgBlue)
		eg.Go(func() error {
			if o.UpdateSnapshot {
				err := os.Remove(charts.SnapshotFile(ht.Chart, ht.ValuesFile))
				if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to replace snapshot file: %w", err)
				}
			}
			matched, failureMessage, err := charts.Snap(ctx, ht)
			if err != nil {
				bannerPrintln("FAIL", fmt.Sprintf("%v chart=%s values=%s", err, ht.Chart, ht.ValuesFile), color.FgRed, color.BgRed)
				return fmt.Errorf("failed to get snapshot: %w chart=%s values=%s", err, ht.Chart, ht.ValuesFile)
			}
			if !matched {
				bannerPrintln("FAIL", failureMessage, color.FgRed, color.BgRed)
				return fmt.Errorf("not match snapshot chart=%s values=%s", ht.Chart, ht.ValuesFile)
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}
	bannerPrintln("PASS", "Snapshot matched", color.FgGreen, color.BgGreen)

	return nil
}

func bannerPrintln(banner string, message string, fgColor color.Attribute, bgColor color.Attribute) {
	color.New(color.FgWhite, bgColor).Printf(" %s ", banner)
	color.New(fgColor).Printf(" %s\n", message)
}

func printOptions(o option) []any {
	rv := reflect.ValueOf(o)
	rt := rv.Type()
	options := make([]any, rt.NumField()*2)

	for i := 0; i < rt.NumField(); i++ {
		options[i*2] = rt.Field(i).Name
		options[i*2+1] = rv.Field(i).Interface()
	}

	return options
}
