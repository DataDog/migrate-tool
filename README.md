# migrate-tool

This tool allows to backup, patch and update Datadog Dashboards and Monitors.
The tool is `Go` binary that can built with:

```
go build -o migrate
```

Usage summary:
```
Usage:
  migrate [flags]
  migrate [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  dump        Dump all specified datadog objects in input files
  help        Help about any command
  patch       Patch all specified datadog objects in input files using selected patcher
  update      Update all (touched) files in input directory

Flags:
  -c, --config string   Path to the config file (default "config.json")
  -h, --help            help for migrate

Use "migrate [command] --help" for more information about a command.
```

The tool **requires a configuration file** (default `config.json`), with credentials per org.
For instance for an org with ID `3000`:
```
{
    "credentials": {
        "3000": {
            "apiKey": "<org_api_key>",
            "appKey": "<org_app_key>"
        }
    }
}
```

## Fetch/backup with `dump`

The `dump` will fetch all specified objects in input files:

```
Dump all specified datadog objects in input files

Usage:
  migrate dump [input files] [flags]

Flags:
  -d, --dashboards string   Path to the dashboard source file
  -h, --help                help for dump
  -m, --monitors string     Path to the monitor source file
  -o, --output string       Output folder (default "objects")
  -u, --update-existing     Update existing objects from Datadog API

Global Flags:
  -c, --config string   Path to the config file (default "config.json")
```

Example:
```
./migrate dump -d my_dashboards.json -m my_monitors.json
```

Input files are JSON files with a list of objects to fetch, for instance:
```
[
    {
        "ORG_ID": 3000,
        "MONITOR_ID": 123456
    },
    {
        "ORG_ID": 3000,
        "DASHBOARD_ID": "foo-bar-baz"
    }
]
```

The `dump` command will create a folder `objects` (`-o/--output`) with the following structure:
```
objects
├── 3000 // Org ID
│   ├── monitor-123456.json
│   └── dashboard-foo-bar-baz.json
```

`dump` can be run multiple times, even with overlapping input content, it will not overwrite existing files unless the `-u / --update-existing` flag is set.

## Patch with `patch`

The `patch` command will patch all objects in input directory:

```
Patch all specified datadog objects in input files using selected patcher

Usage:
  migrate patch [input files] [flags]

Flags:
  -h, --help             help for patch
  -i, --input string     Input folder (default "objects")
  -p, --patcher string   Name of the patcher to use

Global Flags:
  -c, --config string   Path to the config file (default "config.json")
```

Example:
```
./migrate patch -p ksm-to-core
```

For any file modified by the `patch` command, a `.touched` file will be created in the same directory.
It will be used by the `update` command to only update touched files.

Currently only one patcher is available: `ksm-to-core`.

### `ksm-to-core` patcher

The `ksm-to-core` patcher will patch all monitors and dashboards to work with the changes required to migrate from KSM to KSM Core.

The patcher implements most of the changes described in the [migration guide](https://docs.datadoghq.com/integrations/kubernetes_state_core/?tab=helm#migration-from-kubernetes_state-to-kubernetes_state_core).

For `monitors`, it will modify:
* Query
* Name
* Message

For `dashboards`, it will modify:
* Queries
* Template variables used in `kubernetes_state` queries (changing `tag`, not `name`)

## Update with `update`

The `update` command will update all touched files in input directory:

```
Update all (touched) files in input directory

Usage:
  migrate update [input files] [flags]

Flags:
  -h, --help           help for update
  -i, --input string   Input folder (default "objects")
  -u, --update-all     Update all files, not just touched ones

Global Flags:
  -c, --config string   Path to the config file (default "config.json")
```

Example:
```
./migrate update
```

The `update` command will only update files that have been touched by the `patch` command, unless the `-u / --update-all` flag is set.

# Recommended workflow

At first, run the workflow with a **single or a couple of input objects**, then re-run it with all objects.

1. Run `dump` to backup all objects to migrate. 
2. Commit the `objects` folder with a dedicated Git commit.
3. Run `patch` to patch all objects. Use your favorite `git diff` tool to review the changes.
4. Run `update` to update all touched files.

In case of issues detected after the `update` command. You can checkout the `objects` folder from the previous commit (original dump), and run `update` again.

As the `.touched` files are not committed, the `update` command will only update the files that have been touched by the `patch` command.
