# promutil

**promutil** provides a set of utilities for working with a Prometheus 
TSDB.  It allows data generation, recording rule backfilling, data 
migration, etc.

## Building from source

To build **promutil** from the source code yourself you need to have a working
Go environment with [version 1.18 or greater installed](http://golang.org/doc/install).

```console
$ git clone https://github.com/kadaan/promutil.git
$ cd promutil
$ ./build.sh
```

## Usage

```console
promutil provides a set of utilities for working with a Prometheus 
TSDB.  It allows data generation, recording rule backfilling, data 
migration, etc.

Usage:
  promutil [command]

Available Commands:
  backfill    Backfill prometheus recording rule data
  compact     Compact prometheus TSDB
  completion  Output shell completion code for the specified shell (bash or zsh)
  generate    Generate prometheus data
  help        Help about any command
  migrate     Migrate prometheus data
  version     Prints the promutil version
  web         Runs an API/UI server

Flags:
      --config string   config file (default is .promutil.config)
  -h, --help            help for promutil
  -v, --verbose count   enables verbose logging (multiple times increases verbosity)

Use "promutil [command] --help" for more information about a command.

```

### Version

##### Help
```console
$ ./promutil help version
Prints the promutil version

Usage:
  promutil version [flags]

Flags:
  -h, --help   help for version

Global Flags:
      --config string   config file (default is .promutil.config)
  -v, --verbose count   enables verbose logging (multiple times increases verbosity)
```

##### Example
```console
$ ./promutil version
promutil, version v0.0.3-dirty (branch: develop, revision: f49594341b3045ff6d0da7605dab023a1784f201)
  build user:       user@computer.local
  build date:       2022-07-15T05:50:04Z
  go version:       go1.18.3
```

### Backfill

##### Help
```console
$ ./promutil help backfill
Backfill prometheus recording rule data from the specified rules to a local prometheus TSDB.

Usage:
  promutil backfill [flags]

Flags:
      --directory string                  directory read and write TSDB data (default "data/")
      --end timestamp                     time to backfill to (default "now")
  -h, --help                              help for backfill
      --parallelism uint8                 parallelism for backfill (default 16)
      --rule-config-file recordingRules   config file defining the rules to evaluate (default None)
      --rule-group-filter regex           rule group filters which determine the rules groups to backfill (default .+)
      --rule-name-filter regex            rule name filters which determine the rules groups to backfill (default .+)
      --sample-interval duration          interval at which samples will be backfilled (default 15s)
      --start timestamp                   time to backfill from (default "6 hours ago")

Global Flags:
      --config string   config file (default is .promutil.config)
  -v, --verbose count   enables verbose logging (multiple times increases verbosity)
```

##### Example
```console
$ cat recording_rules.yml
groups:
  - name: my rules
    interval: 15s
    rules:
      - record: job:my_service:count
        expr: count by(job) (up{service="my_service"})
      - record: endpoint:http_request_duration_seconds_count:rate1m
        expr: sum by(endpoint, job) (rate(http_request_duration_seconds_count[1m]))
```

```console
$ ./promutil backfill \
      --directory docker/prometheus/data \
      --start 2022-06-18 \
      --end 2022-06-28 \
      --rule-config-file recording_rules.yml \
      --rule-name-filter "job:my_service:count"
Running backfill for 'job:my_service:count' from 2022-06-17T17:00:00 to 2022-06-17T17:29:59
...
```

### Compact

##### Help
```console
$ ./promutil help compact
Compact the specified local prometheus TSDB.

Usage:
  promutil compact [flags]

Flags:
      --directory string   directory read and write TSDB data (default "data/")
  -h, --help               help for compact

Global Flags:
      --config string   config file (default is .promutil.config)
  -v, --verbose count   enables verbose logging (multiple times increases verbosity)
```

##### Example
```console
$ ./promutil compact --directory docker/prometheus/data
Compacting data
```

### Generate

##### Help
```console
$ ./promutil help generate
Generate prometheus data based on the provided data definitions to a local prometheus TSDB.

Usage:
  promutil generate [flags]

Flags:
      --end timestamp                     time to generate data to (default "now")
  -h, --help                              help for generate
      --metric-config-file metricConfig   config file defining the time series to create (default Empty)
      --output-directory string           output directory to write TSDB data (default "data/")
      --parallelism uint8                 parallelism for generation (default 16)
      --rule-config-file recordingRules   config file defining the rules to evaluate (default None)
      --sample-interval duration          interval at which samples will be generated (default 15s)
      --start timestamp                   time to generate data from (default "6 hours ago")

Global Flags:
      --config string   config file (default is .promutil.config)
  -v, --verbose count   enables verbose logging (multiple times increases verbosity)
```

##### Example
```console
$ cat metric_config.yml
timeSeries:
  - name: my_metric
    labels:
      - job: my_job
        service: my_service
    expression: '500'
  - name: my_other_metric
    instances:
      - instance_1
      - instance_2
    labels:
      - job: my_job
        service: my_service
    expression: '500'
```

```console
$ ./promutil generate --start 2022-06-18 --end 2022-06-28 --output-directory docker/prometheus/data --metric-config-file metric_config.yml
Running generate for 'my_metric' from 2022-06-17T17:00:00 to 2022-06-17T17:29:59
Running generate for 'my_other_metric' from 2022-06-17T17:00:00 to 2022-06-17T17:29:59
...
```

### Migrate

##### Help
```console
$ ./promutil help migrate
Migrate the specified data from a remote prometheus to a local prometheus TSDB.

Usage:
  promutil migrate [flags]

Flags:
      --end timestamp              time to migrate to (default "now")
  -h, --help                       help for migrate
      --host url                   remote host to migrate data from (default "http://localhost:9090")
      --matcher matchers           config file defining the rules to evaluate (default None)
      --output-directory string    directory write TSDB data (default "data/")
      --parallelism uint8          parallelism for migration (default 4)
      --sample-interval duration   interval at which samples will be migrated (default 15s)
      --start timestamp            time to migrate from (default "6 hours ago")

Global Flags:
      --config string   config file (default is .promutil.config)
  -v, --verbose count   enables verbose logging (multiple times increases verbosity)

```

##### Example
```console
$ ./promutil migrate --host http://prometheus:9090 --start 2022-06-18 --end 2022-06-28 --output-directory docker/prometheus/data --matcher 'my_metric{service="my_service"} --matcher 'my_other_metric{service="my_service"}'
Running migrate for 'my_metric{service="my_service"}' from 2022-06-17T17:00:00 to 2022-06-17T17:29:59
Running migrate for 'my_other_metric{service="my_service"}' from 2022-06-17T17:00:00 to 2022-06-17T17:29:59
...
```

## Changelog

### 0.0.3
* Added migrate, backfill, generate, compact, and web commands

### 0.0.1
* Initial version

## License

Apache License 2.0, see [LICENSE](https://github.com/kadaan/consulate/blob/master/LICENSE).
