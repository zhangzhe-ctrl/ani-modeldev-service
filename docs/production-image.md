# 生产主服务镜像

在具备 Docker/Buildx 的构建机执行：

```bash
make image IMAGE=10.10.1.66:30003/ani-gpu-platform/ani-modeldev-service:my-tag VERSION=$(git rev-parse HEAD)
```

命令只构建并加载本地镜像；推送、集群发布和质量检查分别执行。
`PLATFORM` 默认 `linux/amd64`，支持 `linux/arm64`；`TARGET` 为 `service`。
可通过 `BUILDER_IMAGE`、`RUNTIME_IMAGE` 和 `GOPROXY` 指定内网镜像代理及固定 digest。
默认 Go 1.26.10，运行层为带系统 CA 的 distroless static Debian 12。

构建显式编译 `cmd/ani-modeldev-service`，不运行代码生成、不连接数据库。
Docker 输入是隔离快照：生产 Go 源码、依赖锁文件、模板 notice，以及
`contract/cpup01/implementations.json`（Go embed 所需）。配置、凭据、Git、
测试和缓存不会传入构建上下文。Go 依赖与编译缓存可跨构建复用。

运行合同：非 root `10001:10001`，入口 `/app/bin/server`，默认参数
`-conf /app/configs`，端口声明 `9000/9001/9002`。运行时必须挂载真实配置及其引用的
证书、凭据等文件；镜像不内置部署配置。示例启动形式：

```bash
docker run --rm my-image:my-tag -h
```

每次构建输出 JSON 回执，保存在 `.work/images/image-*/result.json`，记录 Git revision、
dirty 状态、实际输入 snapshot、基础镜像、平台及本地镜像 digest。本地 digest 不代表
已经推送 Harbor。导出的无 Git 源码需显式使用脚本的 `--source-revision` 参数。

`make test-image` 仅测试镜像编排的外部边界。完整仓库质量检查为
`python3 scripts/with-test-postgres.py make verify`，使用独立受限 PostgreSQL，
不连接业务库；镜像构建成功不能替代质量检查或真实业务验收。

ani-deploy 的 ModelDev 项目对应 `ani-system/ani-modeldev` 的 `modeldev` 容器，
Harbor 路径为 `ani-gpu-platform/ani-modeldev-service`。可选择是否运行质量检查；
发布仅更新主服务镜像，保留现有 `-conf /etc/modeldev/config.yaml`、挂载与 Service。
step 镜像、训练镜像、webhook、数据库和材料生成分别沿用各自发布流程。

根目录 [Dockerfile](../Dockerfile) 是源码到主服务镜像的正式入口。
`deploy/service.Dockerfile` 是历史预编译二进制打包配方，现有脚本使用它时仍可独立运行。
