# sls 

sls stands for Simple Logging Service. It's a server with an API that
aggregates and tails log information and routinely backs up logs to a storage
system.

### API

```
POST /log
Header:
	Body-Content: application/json

Body:
	{
		"Server": "foo",
		"Logs": ["any content at all"]
	}
```

sls takes logs from the request body and appends them to a single file stored
locally on disk allowing for quick local searches with something like
`ripgrep`. It's important to have enough HDD space for that aggregate logfile.
sls can sync changes to that file periodically offsite to a cloud storage
system, like a Google Cloud bucket.

`Server` is an arbitrary identifier. It could be the name of the machine, the
name of the application that's running on it, the IP address, or some
combination of those.

On ordering: it's assumed that logs batched together in a single API call are
from the same request or are related to the same action. Multiple requests
(e.g. logs from several different HTTP requests) can be included in a single
API call, but to ensure proper ordering, do not split up a request across API
calls. A simple buffer that flushes to the API is not sufficient if ordering
needs to be preserved.

On timestamps: sls does not add timestamps to the logs. If needed, servers
doing the logging should include them.

`GET /log` will `tail` the logs out as they're received by sls and processed,
so you'll see:

```
{"Server":"foo","Log":"user 2 made request"}
{"Server":"foo","Log":"user 2 request complete"}
{"Server":"bar","Log":"user 1 deleted photo"}
{"Server":"foo","Log":"user 2 logged out"}
```

Streamed to your terminal for real-time, distributed debugging using `curl`.
You can use pipes and tools like `jq` to filter only what you need, like so:

```
$ curl /log | grep "foo" | jq .Log
```

### Getting meta

sls writes its own internal logs to an io.Writer (like stdout), allowing us to
inspect what's going wrong with sls if it can't for some reason write to the
logfile or backups to the offsite storage fail.

### Design goals

* It should be simple and easy to grok with a small codebase to minimize bugs
* Allow for very fast log searching using other tools
* Aggregate tailing of logs in the command line
