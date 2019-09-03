Json api
----

本服务可以通过请求Json api来更新配置：

安装
----

```sh
git clone github.com/mapleque/proxy
cd proxy
make -I plugin/jsonapi
```

该命令会在当前路径下生成一个`bin/jsonapi`的可执行文件，根据[.env.example](../../../.env.example)配置好环境变量后执行它即可启动。

开发环境可以直接启动服务：
```sh
make run -I plugin/jsonapi
```


```sh
curl -X POST -d '@app.json' http://127.0.0.1::9999/app/add
```

其中app.json的内容格式，参考[proxy配置说明](../../../#app)。

服务启动需要配置环境变量：

```sh
DEBUG=false
JSON_API_LISTEN=9999
JSON_API_TOKEN=a_secret_token
PROXY_CONFIG_FILE_PATH=/path/to/proxy/config/
PROXY_PID_FILE=/path/to/proxy.pid
RELOAD_LAZY_SECONDS=60
LOG_DIR=/path/to/log/
```
