# PM2-GO

[![Go Report Card](https://goreportcard.com/badge/github.com/dunstorm/pm2-go)](https://goreportcard.com/report/github.com/dunstorm/pm2-go)

PM2-GO is a clone of [Unitech/pm2](https://github.com/Unitech/pm2) made using golang. The aim is to make it easy to install. Performance is the bonus.

Starting an application in production mode is as easy as:

```
pm2-go start python app.py
```

Works on Linux & macOS, no support for Windows.

## Start an application

You can start any application (Node.js, Python, Ruby, binaries in $PATH...) like that:

```
pm2-go start python app.py
```

## Managing Applications

Once applications are started you can manage them easily:

To list all running applications:

```
pm2-go ls
```

Managing apps is straightforward:

```
pm2-go stop     <app_name|id|json_conf|'all'>
pm2-go restart  <app_name|id|json_conf|'all'>
pm2-go delete   <app_name|id|json_conf|'all'>
pm2-go flush    <app_name|id|json_conf|'all'>
```

To see real-time logs:

```
pm2-go logs <app_name|id>
```

To dump all processes

```
pm2-go dump
```

To restore all processes

```
pm2-go restore
```

## Extend Logs

Logs can be extended by using `scripts` placed in `$HOME/.pm2-go/scripts`.

To leverage the use of `scripts`, specify the `scripts` inside `.json` file containing processes

e.g.

```json
[
    {
        "name": "python-test",
        "args": ["-u", "test.py"],
        "autorestart": true,
        "cwd": "./examples",
        "scripts": ["logs_date"],
        "executable_path": "python3",
        "cron_restart": "* * * * *"
    }
]
```

Example script: [logs_date.sh](https://github.com/dunstorm/pm2-go/blob/main/scripts/logs_date.sh)

It adds corresponding date & time to each line from the process output

![](https://i.imgur.com/UFw9PpX.png)