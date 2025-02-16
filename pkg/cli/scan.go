package cli

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/samber/lo"
	"github.com/savioxavier/termlink"
	"github.com/spf13/cobra"
	"github.com/wolfi-dev/wolfictl/pkg/scan"
)

func Scan() *cobra.Command {
	p := &scanParams{}
	cmd := &cobra.Command{
		Use:           "scan <path/to/package.apk> ...",
		Short:         "Scan an apk file for vulnerabilities",
		Args:          cobra.MinimumNArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				apkFilePath := arg
				apkFile, err := os.Open(apkFilePath)
				if err != nil {
					return fmt.Errorf("failed to open apk file: %w", err)
				}

				fmt.Println(path.Base(apkFilePath))

				findings, err := scan.APK(apkFile)
				if err != nil {
					return err
				}

				apkFile.Close()

				if len(findings) == 0 {
					fmt.Println("✅ No vulnerabilities found")
				} else {
					tree := newFindingsTree(findings)
					fmt.Println(tree.render())
				}

				if p.requireZeroFindings && len(findings) > 0 {
					return fmt.Errorf("more than 0 vulnerabilities found")
				}
			}

			return nil
		},
	}

	p.addFlagsTo(cmd)
	return cmd
}

type scanParams struct {
	requireZeroFindings bool
}

func (p *scanParams) addFlagsTo(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&p.requireZeroFindings, "require-zero", false, "exit 1 if any vulnerabilities are found")
}

type findingsTree struct {
	findingsByPackageByLocation map[string]map[string][]*scan.Finding
	packagesByID                map[string]scan.Package
}

func newFindingsTree(findings []*scan.Finding) *findingsTree {
	tree := make(map[string]map[string][]*scan.Finding)
	packagesByID := make(map[string]scan.Package)

	for _, f := range findings {
		loc := f.Package.Location
		packageID := f.Package.ID
		packagesByID[packageID] = f.Package

		if _, ok := tree[loc]; !ok {
			tree[loc] = make(map[string][]*scan.Finding)
		}

		tree[loc][packageID] = append(tree[loc][packageID], f)
	}

	return &findingsTree{
		findingsByPackageByLocation: tree,
		packagesByID:                packagesByID,
	}
}

func (t findingsTree) render() string {
	locations := lo.Keys(t.findingsByPackageByLocation)
	sort.Strings(locations)

	var lines []string
	for i, location := range locations {
		var treeStem, verticalLine string
		if i == len(locations)-1 {
			treeStem = "└── "
			verticalLine = " "
		} else {
			treeStem = "├── "
			verticalLine = "│"
		}

		line := treeStem + fmt.Sprintf("📄 %s", location)
		lines = append(lines, line)

		packageIDs := lo.Keys(t.findingsByPackageByLocation[location])
		packages := lo.Map(packageIDs, func(id string, _ int) scan.Package {
			return t.packagesByID[id]
		})

		sort.SliceStable(packages, func(i, j int) bool {
			return packages[i].Name < packages[j].Name
		})

		for _, pkg := range packages {
			line := fmt.Sprintf(
				"%s       📦 %s %s %s",
				verticalLine,
				pkg.Name,
				pkg.Version,
				styleSubtle.Render("("+pkg.Type+")"),
			)
			lines = append(lines, line)

			findings := t.findingsByPackageByLocation[location][pkg.ID]
			sort.SliceStable(findings, func(i, j int) bool {
				return findings[i].Vulnerability.ID < findings[j].Vulnerability.ID
			})

			for _, f := range findings {
				line := fmt.Sprintf(
					"%s           %s %s%s",
					verticalLine,
					renderSeverity(f.Vulnerability.Severity),
					renderVulnerabilityID(f.Vulnerability),
					renderFixedIn(f.Vulnerability),
				)
				lines = append(lines, line)
			}
		}

		lines = append(lines, verticalLine)
	}

	return strings.Join(lines, "\n")
}

func renderSeverity(severity string) string {
	switch severity {
	case "Negligible":
		return styleNegligible.Render(severity)
	case "Low":
		return styleLow.Render(severity)
	case "Medium":
		return styleMedium.Render(severity)
	case "High":
		return styleHigh.Render(severity)
	case "Critical":
		return styleCritical.Render(severity)
	default:
		return severity
	}
}

func renderVulnerabilityID(vuln scan.Vulnerability) string {
	var cveID string

	for _, alias := range vuln.Aliases {
		if strings.HasPrefix(alias, "CVE-") {
			cveID = alias
			break
		}
	}

	if cveID == "" {
		return hyperlinkVulnerabilityID(vuln.ID)
	}

	return fmt.Sprintf(
		"%s %s",
		hyperlinkVulnerabilityID(cveID),

		styleSubtle.Render(hyperlinkVulnerabilityID(vuln.ID)),
	)
}

var termSupportsHyperlinks = termlink.SupportsHyperlinks()

func hyperlinkVulnerabilityID(id string) string {
	if !termSupportsHyperlinks {
		return id
	}

	switch {
	case strings.HasPrefix(id, "CVE-"):
		return termlink.Link(id, fmt.Sprintf("https://nvd.nist.gov/vuln/detail/%s", id))

	case strings.HasPrefix(id, "GHSA-"):
		return termlink.Link(id, fmt.Sprintf("https://github.com/advisories/%s", id))
	}

	return id
}

func renderFixedIn(vuln scan.Vulnerability) string {
	if vuln.FixedVersion == "" {
		return ""
	}

	return fmt.Sprintf(" fixed in %s", vuln.FixedVersion)
}

var (
	styleSubtle = lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))

	styleNegligible = lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	styleLow        = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00"))
	styleMedium     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffff00"))
	styleHigh       = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff9900"))
	styleCritical   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
)
