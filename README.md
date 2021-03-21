# awsputlogs

awsputlogs is tool to upload JSON and string logs to the AWS CloudWatch Logs easily.

## Usage

Upload log events

```bash
$ awsputlogs --log-group <LOG GROUP NAME> "sample log message1" "sample log message2"
```

Upload log events to specified log stream

```bash
$ awsputlogs --log-group <LOG GROUP NAME> --log-stream <LOG STREAM NAME> "sample log message1"
```

You should use '--logs-file' option if you want to upload JSON logs or many logs.

```bash
$ awsputlogs --log-group <LOG GROUP NAME> --logs-file <FILE PATH>
```

You must write a file with the following formats.

Upload JSON logs

```json
[
    {
        "level": "info",
        "message": "Start Server"
    },
    {
        "level": "error",
        "message": "Failed to Start Server"
    }
]
```

Upload string logs

```json
[
    "[INFO] Start Server",
    "[ERROR] Failed to Start Server"
]
```

Upload JSON and string logs

```json
[
    {
        "level": "info",
        "message": "Start Server"
    },
    "[ERROR] Failed to Start Server"
]
```

## LICENCE

MIT
