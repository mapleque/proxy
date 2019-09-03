一个Http代理服务
====

这是一个http代理服务，主要用于在服务器上实现反向代理和负载均衡。在负载均衡中，用户可配置健康检查用于提高可用性。

此外，该服务可以使用以下插件用于生产环境部署：

- [Json Api](./plugin/jsonapi) 提供基于json格式的http接口，用于管理配置
- [K8S Watcher](./plugin/k8swatcher) 提供watch k8s api server的能力以同步更新配置

如何开始
----

- 启动： `proxy -c /path/to/your/config/file.json`
- 停止： `proxy -s stop`
- 重新加载配置： `proxy -s reload`

使用`proxy -h`命令可以查看更多可用参数：

工作流
----

- 端口接收到请求检查证书
- 查找url和header匹配的rule
- 进行必要的request header转换
- 替换代理目标地址中的变量
- 请求代理目标接收返回数据
- 进行必要的response header转换
- 返回数据
- 写日志

基本配置
----

本服务配置可以通过一个json格式的字符串给出，字段说明如下：
```json
{
  "apps": [<app>, ...],
  "services": [<service>, ...],
  "logfmts": [<logfmt>, ...],
  "syslog": <log>
}
```

其中，
- apps 必选，用于配置应用。`<app>`是一个应用的配置，字段说明参考[app](#app)章节。
- services 可选，用于配置全局服务集。`<service>`是一个服务集的配置，字段说明参考[service](#service)章节。
- logfmts 可选，用于定义全局日志格式。`<logfmt>`是一个日志格式定义的配置，字段说明参考[logfmt](#logfmt)章节。
- syslog 可选，用于配置系统日志输出，默认输出到标准输出。`<log>`是一个日志输出规则的配置，字段说明参考[log](#log)章节。

> 特别的：配置文件的任何字段值都可以通过引用其他文件来定义(`"@include [a-zA-Z0-9_/\.\*]+\.json"`)，如：
>
> `config.json`:
>
> ```json
> {
>   "apps": "@include apps/*.json",
>   "logfmts": [
>     "@include logfmt.json"
>   ]
> }
> ```
>
> `apps/a.json`:
>
> ```json
> {
>   "port": 80,
>   "rules": ...
> }
> ```
>
> ```json
> {
>   "name": "access",
>   "lines": [ "$request_start [REQ] this is a request log" ]
> }
> ```
>
> 注意：当文件名中含有`*`时，这个`@include`将被组成数组数据。

app
----

应用配置，字段说明如下：

```json
{
  "port": <port>,
  "certfile": "/absolute/path/of/cert/file.cert",
  "keyfile": "/absolute/path/of/key/file.key",
  "domains": [<domain>, ...],
  "services": [<service>, ...],
  "access_log": <log>,
  "error_log": <log>,
  "logfmts": [<logfmt>, ...]
}
```

其中，
- port 必选，用于配置当前应用所监听的本机端口。`<port>`是一个端口，如：`80,8080`等。
- certfile 可选，用于配制https证书。这里建议填写操作系统绝对路径。
- keyfile 可选，用于配制https证书。这里建议填写操作系统绝对路径。
- domains 必选，用于配置当前应用的域名。`<domain>`包含一个域名名称以及与该域名相关的规则的配置，字段说明参考[domain](#domain)章节。
- services 可选，用于配置当前应用的服务集，这里配置的服务集仅在当前应用可见，同名配置会覆盖全局中的定义。`<service>`是一个服务集的配置，字段说明参考[service](#service)章节。
- access_log 可选，用于配置当前应用输出请求日志的规则。`<log>`是一个日志输出规则的配置，字段说明参考[log](#log)章节。
- error_log 可选，用于配置当前应用输出错误日志的规则。`<log>`是一个日志输出规则的配置，字段说明参考[log](#log)章节。
- logfmts 可选，用于定义当前应用的日志格式，这里配置的日志格式仅当前应用可见，同名配置会覆盖全局中的定义。`<logfmt>`是一个日志格式定义的配置，字段说明参考[logfmt](#logfmt)章节。

domain
----

规则配置，字段说明如下：

```json
{
  "domain": "source.domain.name",
  "rules": [<rule>, ...],
}
```

其中，
- domain 必选，用于指定该domain的域名, 匹配该域名的请求将会使用该domain的rule进行匹配, 寻找目标请求地址。
- rules 必选，用于配置当前应用的规则。`<rule>`是一个规则的配置，字段说明参考[rule](#rule)章节。

rule
----

规则配置，字段说明如下：

```json
{
  "filters": [<filter>, ...],
  "to": <to_url>,
  "transform": {
    "headers": [<header_transform>, ...]
  }
}
```

其中，
- filters 可选，用于辨识请求来源，数组为空或满足数组中任一条件的请求都将适用当前规则。`<filter>`是一个便是请求来源的配置，字段说明参考[filter](#filter)。
- to 必选，用于指定目标请求地址。`<to_url>`是一个请求地址字符串，支持的格式定义为：`[schema://(host[:port]|service.name)[/path]]`。可以使用变量，参考[变量说明](#变量说明)章节。
- transform 可选，用于改变请求和返回的数据。
- transform.headers 可选，用于指定要修改的header属性。`<header_transform>`是一个修改header的配置，规则参考[header_transform](#header_transform)。

filter
----

辨识请求来源配置，字段说明如下：

```json
{
  "request_uris": [<request_uri_filter>, ...],
  "headers": [<header_filter>, ...]
}
```

其中：
- request_uris 可选，用于指定来源请求地址，多项规则之间关系为“或”。`<request_uri_filter>`是一个正则表达式，原请求将以以下格式定义进行正则匹配： `[/request_uri]`。
- headers 可选，用于指定header需要满足的条件，多项规则之间关系为“或”。`<header_filter>`是一个header过滤条件的配置，规则说明参考[header_filter](#header_filter)。

> 注意：以上每个字段之间关系为“且”。


header_filter
----

header过滤条件配置，字段说明如下：

```json
{
  "key": <Http Header Key>,
  "value": <Http Header Value>
}
```

其中，
- key 必选，操作的Http Header的键。
- value 必选，操作的Http Header的值，可以使用变量，参考[变量说明](#变量说明)章节。

header_transform
----

修改header配置，字段说明如下：

```json
{
  "when": <request|response>,
  "method": <add|set|del>,
  "key": <Http Header Key>,
  "value": <Http Header Value>,
  "pattern": <regexp>
}
```

其中，
- when 必选，用于指定是修改request还是response中的Http Header。
- method 必选，用于指定进行何种操作，add为追加、set为替换、del为删除。
- key 必选，目标的Http Header的键。
- value 可选，目标的Http Header的值，当method为del时无需添加该字段。可以使用变量，参考[变量说明](#变量说明)章节。
- pattern 可选，内容为正则表达式, 如果pattern匹配key所指向的Header的内容, 则提取匹配的值用于后续使用, 如果没有匹配则停止执行, 不填写则不检查匹配。

service
----

服务集配置，字段说明如下：

```json
{
  "name": <[a-z]+[a-z0-9_]*>,
  "hosts": [<host>, ...],
  "checks": [<check>, ...]
}
```

其中，
- name 必选，服务集名称。首字母为a-z的小写字母，其他为小写字母数字和下划线。
- hosts 必选，服务集所包含的服务。`<host>`是一个服务配置，字段说明参考[host](#host)。
- checks 可选，服务集健康检查。`<check>`是一个健康检查配置，字段说明参考[check](#check)。

host
----

服务配置，字段说明如下：

```json
{
  "host": <(ip|domain)[:port]>,
  "weight": <1-100>,
  "checks": [<check>, ...]
}
```

其中，
- host 必选，服务的地址，可以是ip或域名，可以带端口。
- weight 必选，服务的负载均衡权重，为1-100的正整数，是个相对值。
- checks 可选，服务健康检查。`<check>`是一个健康检查配置，字段说明参考[check](#check)。

负载均衡权重将会在服务集中发挥作用，当前服务被请求的概率为当前服务权重与服务集中所有服务权重之和的百分比。

check
----

健康检查配置，字段说明如下：

```json
{
  "schema": <http|https>,
  "path": "/health/check/api",
  "method": <POST|GET>,
  "interval": <1-3600>,
  "timeout": <1-3600>,
  "status": 200,
  "body": "success",
  "window": <1-3600>,
  "down": <1-3600>,
  "up": <1-3600>
}
```

其中，
- schema 可选，健康检查请求的协议，默认用http。
- path 必选，健康检查请求的地址。
- method 必选，健康检查请求的Http Method。
- interval 必选，健康检查请求发送频率，整数，单位为秒。
- timeout 可选，健康检查请求超时时间，整数，单位为秒。
- status  可选，健康检查要求返回的Http Status。
- body 可选，健康检查要求返回体所包含的内容。
- window 必选，健康检查统计窗口次数。
- down 必选，健康检查摘掉服务的窗口内失败次数阈值。
- up 必选，健康检查恢复服务的窗口内成功次数阈值。

> 注意，这里必须满足down+up > window

log
----

日志输出规则配置，字段说明如下：

```json
{
  "file": "/absolute/path/of/log/file.log",
  "fmt": <logfmt name>,
  "rotate_time": <hour|day>,
  "rotate_size": <1-1024*1024*1024>,
  "rotate_number": <1-1024>
}
```

其中，
- file 必选，用于指定输出日志文件，这里需要填写操作系统绝对路径，并且赋予当前服务写文件的权限。
- fmt 必选，用于指定日志输出格式，输出格式必须是已经定义过的logfmt的name，参考[logfmt](#logfmt)。
- rotate_time 可选，用于指定按时间切分日志，支持：hour按小时、day按日、week按周。
- rotate_size 可选，用于指定按大小切分日志，整数，单位为字节。
- rotate_number 可选，用于指定切分日志文件保留的个数，超出的文件将会被按照切分时间先后顺序删除。

logfmt
----

日志输出格式定义，字段说明如下：

```json
{
  "name": <[a-z]+[a-z0-9_]*>,
  "lines": [<line>, ...]
}
```
其中，
- name 必选，日志输出格式定义名称，用于日志输出配置中指定格式。首字母为a-z的小写字母，其他为小写字母数字和下划线。
- lines 必选，日志输出的格式定义，可以定义多行输出。每一行`<line>`都可以通过字符加变量排布的形式定义日志格式，可以使用的变量参考[变量说明](#变量说明)章节。

> 例如：
>
> ```
> $request_start [INFO] $remote_ip $request_end $latency $method $host $uri_path $uri_param $status
> ```

变量说明
----

- $n 其中n=1,2,...，上一个正则表达式所获取的值
- $host 请求的host
- $real_host 实际请求的host
- $request_start 请求开始时间，格式：yyyy/MM/dd HH:mm:ss
- $remote_ip 请求方ip
- $request_end 请求返回时间，格式：yyyy/MM/dd HH:mm:ss
- $latency 请求响应时长，整数，单位是毫秒(ms)
- $method http method
- $request_uri 请求的完整uri, 包含path和query等, 包含中间的?
- $uri_path 请求path
- $uri_query 编码的请求参数，不包含?，如果没有则留空
- $status 返回的Http Status
- $x_forward_for 代理后的X-Forward-For
- $header_<key> 指定key的Http Header
- $error_message 错误信息
