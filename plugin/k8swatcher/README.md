K8S Watcher
====

本服务可以配置订阅k8s集群service变化数据，并触发更细配置文件。

安装
----

```sh
git clone github.com/mapleque/proxy
cd proxy
make -I plugin/k8swatcher
```

该命令会在当前路径下生成一个`bin/k8swatcher`的可执行文件，根据[.env.example](../../../..env.example)配置好环境变量后执行它即可启动。

开发环境可以直接启动服务：
```sh
# 启动前请务必创建并认真配置环境变量
make run -I plugin/k8swatcher
```

使用
----

本服务会对`PROXY_CONFIG_FILE_PATH`下的`apps-k8swacher`文件夹中的文件进行增删改查，因此proxy运行时需要在入口配置文件中，包含这两个路径下的所有文件。

```json
{
  "apps": [
    "@include ./apps-k8swacher/*.json"
  ]
}
```

协议
----

使用本服务，需要对k8s的pod配置相关label。
- domain 项目访问的基础域名
- `port_<port_name>` 项目所使用的端口名映射

k8swatcher会读取api返回的resp.object.metadata.labels中的数据进行配置，生成访问域名：
```
[port_name.]domain
```

注意，这里的域名只支持http协议，如果需要https协议，请自行修改插件。

