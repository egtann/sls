# sls 

sls stands for Simple Logging Service. It's a server with an API that
aggregates and tails log information and routinely backs up logs to a storage
system.

### API

```
POST /log
Header:
	Body-Content: application/json
	X-API-Key: your-key-here

Body:
	[
		"any content at all",
		"separated into elements",
		"in a json array",
	]
```

sls takes logs from the request body and appends them to a single file stored
locally on disk allowing for quick local searches with something like
`ripgrep`. It's important to have enough HDD space for those logfiles. sls can
sync changes to that file periodically offsite to a cloud storage system, like
a Google Cloud bucket and clear out local logfiles after a set period of time
has passed.

**Ordering:** it's assumed that logs batched together in a single API call are
from the same request or are related to the same action. Multiple requests
(e.g. logs from several different HTTP requests) can be included in a single
API call, but to ensure proper ordering, do not split up a request across API
calls. A simple buffer that flushes to the API is not sufficient if ordering
needs to be preserved.

**Timestamps:** sls does not add timestamps to the logs. If needed, servers
doing the logging should include them.

**Auth:** sls checks the provided X-API-Key header to make sure it matches the
API_KEY provided in the configuration file.

`GET /log` will `tail` the logs out as they're received by sls and processed,
so you'll see:

```
{"Server":"foo","Msg":"user 2 made request"}
{"Server":"foo","Msg":"user 2 request complete"}
{"Server":"bar","Msg":"user 1 deleted photo"}
{"Server":"foo","Msg":"user 2 logged out"}
```

Streamed to your terminal for real-time, distributed debugging using `curl`.
You can use pipes and tools like `jq` to filter only what you need, like so:

```
$ curl -sN /log | grep '"Server":"foo"' | jq .Msg
```

Note that we're using the curl flags `-s` for silent, since we don't want
progress updates, and `-N` to prevent buffering, since we're streaming the
response.

### Configuration

Create `sls.conf` (or use the `-c` flag to specify a custom file), and fill it
out with the following variables:

```
PORT=
DIR=
API_KEY=
RETAIN_FOR_DAYS=
```

Logfiles will be written to the given directory with the format YYYYMMDD.log.

### Getting meta

sls writes its own internal logs, allowing us to inspect what's going wrong
with sls if it can't for some reason write to the logfile or backups to the
offsite storage fail.

### Design goals

sls:

* Is simple and easy to grok with a small codebase to minimize bugs
* Enables tailing of aggregated logs in the command line
* Allows for very fast log searches (using other cli tools)

sls does not have a web interface and does not natively support searching.
