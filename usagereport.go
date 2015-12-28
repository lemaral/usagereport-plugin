package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/cloudfoundry/cli/plugin"
	"github.com/cloudfoundry/cli/cf/formatters"
	"github.com/lemaral/Wildcard_Plugin/table"
	"github.com/lemaral/usagereport-plugin/apihelper"
)

//UsageReportCmd the plugin
type UsageReportCmd struct {
	apiHelper apihelper.CFAPIHelper
	cli       plugin.CliConnection
}

type org struct {
	name        string
	memoryQuota int
	memoryUsage int
	spaces      []space
}

type space struct {
	apps []app
	name string
}

type app struct {
	ram       int
	instances int
	running   bool
}

//GetMetadata returns metatada
func (cmd *UsageReportCmd) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "usage-report",
		Version: plugin.VersionType{
			Major: 1,
			Minor: 1,
			Build: 0,
		},
		Commands: []plugin.Command{
			{
				Name:     "usage-report",
				HelpText: "Report AI and memory usage for orgs and spaces",
				UsageDetails: plugin.Usage{
					Usage: "cf usage-report",
				},
			},
		},
	}
}

type SpaceByName []space
func (a SpaceByName) Len() int           { return len(a) }
func (a SpaceByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SpaceByName) Less(i, j int) bool { return a[i].name < a[j].name }

//UsageReportCommand doer
func (cmd *UsageReportCmd) UsageReportCommand(args []string) {
	tableHeader := []string{("name"), ("apps"), ("instances"), ("memory"), ("mem%quota")}

	if nil == cmd.cli {
		fmt.Println("ERROR: CLI Connection is nil!")
		os.Exit(1)
	}

	orgs, err := cmd.getOrgs()
	if nil != err {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Gathering usage information for", len(orgs), "orgs")

	totalSpaces := 0
	for _, org := range orgs {
		totalApps := 0
		totalRunningApps := 0
		totalInstances := 0
		totalRunningInstances := 0
		var totalMemory int64 = 0

		spaces := org.spaces
		sort.Sort(SpaceByName(spaces))
		var memoryUsage int64 = int64(org.memoryUsage)
		var memoryQuota int64 = int64(org.memoryQuota)
		memoryUsageString := formatters.ByteSize(memoryUsage * formatters.MEGABYTE)
		memoryQuotaString := formatters.ByteSize(memoryQuota * formatters.MEGABYTE)
		var quotaPercentOrg float64 = float64(100.0 * memoryUsage / memoryQuota)
		percentOrgString := fmt.Sprintf("%02.2f%%", quotaPercentOrg);
		fmt.Printf("Org %s is consuming %s of %s (%s) in %d spaces.\n", org.name, memoryUsageString, memoryQuotaString, percentOrgString, len(spaces))
		mytable := table.NewTable(tableHeader)
		for _, space := range spaces {
			var memory int64 = 0
			instances := 0
			runningApps := 0
			runningInstances := 0
			for _, app := range space.apps {
				if app.running {
					memory += int64(app.instances * app.ram)
					runningApps++
					runningInstances += app.instances
				}
				instances += int(app.instances)
			}
			appsString := strconv.Itoa(runningApps) + "/" + strconv.Itoa(len(space.apps))
			instancesString := strconv.Itoa(runningInstances) + "/" + strconv.Itoa(instances)
			memoryString := formatters.ByteSize(memory * formatters.MEGABYTE)
			var quotaPercent float64 = float64(100.0 * memory / int64(org.memoryQuota))
			percentString := fmt.Sprintf("%02.2f%%", quotaPercent);
			mytable.Add(space.name, appsString, instancesString, memoryString, percentString)

			totalApps += len(space.apps)
			totalRunningApps += runningApps
			totalInstances += instances
			totalRunningInstances += runningInstances
			totalMemory += memory
		}
		appsString := strconv.Itoa(totalRunningApps) + "/" + strconv.Itoa(totalApps)
		instancesString := strconv.Itoa(totalRunningInstances) + "/" + strconv.Itoa(totalInstances)
		memoryString := formatters.ByteSize(totalMemory * formatters.MEGABYTE)
		var quotaPercent float64 = float64(100 * totalMemory / int64(org.memoryQuota))
		percentString := fmt.Sprintf("%02.2f%%", quotaPercent);
		mytable.Add("Total", appsString, instancesString, memoryString, percentString)
		mytable.Print()
		totalSpaces += len(spaces)
	}
	fmt.Printf("Total orgs:%d, Total spaces: %d\n", len(orgs), totalSpaces)
}

func (cmd *UsageReportCmd) getOrgs() ([]org, error) {
	rawOrgs, err := cmd.apiHelper.GetOrgs(cmd.cli)
	if nil != err {
		return nil, err
	}

	var orgs = []org{}

	for _, o := range rawOrgs {
		usage, err := cmd.apiHelper.GetOrgMemoryUsage(cmd.cli, o)
		if nil != err {
			return nil, err
		}
		quota, err := cmd.apiHelper.GetQuotaMemoryLimit(cmd.cli, o.QuotaURL)
		if nil != err {
			return nil, err
		}
		spaces, err := cmd.getSpaces(o.SpacesURL)
		if nil != err {
			return nil, err
		}

		orgs = append(orgs, org{
			name:        o.Name,
			memoryQuota: int(quota),
			memoryUsage: int(usage),
			spaces:      spaces,
		})
	}
	return orgs, nil
}

func (cmd *UsageReportCmd) getSpaces(spaceURL string) ([]space, error) {
	rawSpaces, err := cmd.apiHelper.GetOrgSpaces(cmd.cli, spaceURL)
	if nil != err {
		return nil, err
	}
	var spaces = []space{}
	for _, s := range rawSpaces {
		apps, err := cmd.getApps(s.AppsURL)
		if nil != err {
			return nil, err
		}
		spaces = append(spaces,
			space{
				apps: apps,
				name: s.Name,
			},
		)
	}
	return spaces, nil
}

func (cmd *UsageReportCmd) getApps(appsURL string) ([]app, error) {
	rawApps, err := cmd.apiHelper.GetSpaceApps(cmd.cli, appsURL)
	if nil != err {
		return nil, err
	}
	var apps = []app{}
	for _, a := range rawApps {
		apps = append(apps, app{
			instances: int(a.Instances),
			ram:       int(a.RAM),
			running:   a.Running,
		})
	}
	return apps, nil
}

//Run runs the plugin
func (cmd *UsageReportCmd) Run(cli plugin.CliConnection, args []string) {
	if args[0] == "usage-report" {
		cmd.apiHelper = &apihelper.APIHelper{}
		cmd.cli = cli
		cmd.UsageReportCommand(args)
	}
}

func main() {
	plugin.Start(new(UsageReportCmd))
}
