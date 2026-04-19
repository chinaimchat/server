# ── Stage: twemoji72 ──────────────────────────────────────────────────────────
# 在构建镜像时拉取 Twemoji 72×72 PNG 到 /out，供最终镜像内置使用。
# 优先使用仓库内预置的 assets/twemoji72/（完全离线构建场景）；
# 否则通过 git sparse-checkout 精准抽取 72x72 目录（网络传输 ~1.5MB）。
FROM alpine:3.20 AS twemoji72
ARG TWEMOJI_VERSION=14.0.2
ARG TWEMOJI_REPO_URL=https://github.com/twitter/twemoji.git
RUN apk add --no-cache git ca-certificates
WORKDIR /src
# 允许源码树预置 assets/twemoji72/（可选），用于无外网的流水线
COPY assets ./assets
RUN set -eu; \
    mkdir -p /out; \
    if ls ./assets/twemoji72/*.png >/dev/null 2>&1; then \
        echo "using prebundled assets/twemoji72/ from repo"; \
        cp ./assets/twemoji72/*.png /out/; \
    else \
        echo "sparse-checkout twemoji@${TWEMOJI_VERSION}"; \
        git -c advice.detachedHead=false clone \
            --depth=1 --filter=blob:none --sparse \
            --branch="v${TWEMOJI_VERSION}" "${TWEMOJI_REPO_URL}" /tmp/tw; \
        (cd /tmp/tw && git sparse-checkout set --no-cone assets/72x72); \
        cp /tmp/tw/assets/72x72/*.png /out/; \
        rm -rf /tmp/tw; \
    fi; \
    echo "twemoji72 files: $(ls /out | wc -l)"

# ── Stage: build ──────────────────────────────────────────────────────────────
FROM golang:1.20 AS build

ENV GOPROXY=https://goproxy.cn,direct
ENV GO111MODULE=on


WORKDIR /go/cache


ADD go.mod .
ADD go.sum .
RUN go mod download

WORKDIR /go/release



# RUN apt-get update && \
#       apt-get -y install ca-certificates 

ADD . .

# RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-w -extldflags "-static"' -installsuffix cgo -o app ./main.go

RUN GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo unknown) && \
    GIT_COMMIT_DATE=$(git log --date=iso8601-strict -1 --pretty=%ct 2>/dev/null || date +%s) && \
    GIT_VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo dev) && \
    GIT_TREE_STATE=$(if [ -d .git ] && [ -n "$(git status --porcelain 2>/dev/null)" ]; then echo dirty; else echo clean; fi) && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -extldflags '-static' -X main.Commit=$GIT_COMMIT -X main.CommitDate=$GIT_COMMIT_DATE -X main.Version=$GIT_VERSION -X main.TreeState=$GIT_TREE_STATE" -installsuffix cgo  -o app ./main.go


FROM alpine AS prod
# Import the user and group files from the builder.
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
RUN \ 
    mkdir -p /usr/share/zoneinfo/Asia && \
    ln -s /etc/localtime /usr/share/zoneinfo/Asia/Shanghai
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN apk add --no-cache ffmpeg
WORKDIR /home
COPY --from=build /go/release/app /home
COPY --from=build /go/release/assets /home/assets
COPY --from=build /go/release/configs /home/configs
# 内置 Twemoji 72×72 PNG，运行时无需出网即可命中 /v1/common/twemoji72/*
COPY --from=twemoji72 /out/ /home/assets/twemoji72/
ENV TWEMOJI_72_DIR=/home/assets/twemoji72
RUN echo "Asia/Shanghai" > /etc/timezone
ENV TZ=Asia/Shanghai

# 不加  apk add ca-certificates  apns2推送将请求错误
# RUN  apk add ca-certificates 

ENTRYPOINT ["/home/app"]
