FROM golang:1.24.1 AS builder

RUN apt-get update && apt-get install -y \
    build-essential \
    libopenblas-dev \
    wget \
    git \
    libssl-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# 构建新版本的 CMake，以满足 FAISS 的要求
WORKDIR /tmp
RUN wget https://github.com/Kitware/CMake/releases/download/v3.26.5/cmake-3.26.5.tar.gz && \
    tar -zxf cmake-3.26.5.tar.gz && \
    cd cmake-3.26.5 && \
    ./bootstrap --prefix=/usr/local/cmake && \
    make -j$(nproc) && \
    make install

# 将 CMake 添加到 PATH
ENV PATH="/usr/local/cmake/bin:${PATH}"

# 构建 FAISS
WORKDIR /tmp
RUN git clone https://github.com/facebookresearch/faiss.git && \
    cd faiss && \
    cmake -B build . -DFAISS_ENABLE_GPU=OFF -DFAISS_ENABLE_PYTHON=OFF \
        -DBUILD_SHARED_LIBS=ON -DFAISS_ENABLE_TESTING=OFF \
        -DFAISS_BUILD_TESTS=OFF -DBUILD_TESTING=OFF -DFAISS_ENABLE_C_API=ON && \
    make -C build -j$(nproc) && \
    make -C build install && \
    ldconfig

# 设置工作目录
WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# 构建应用
RUN CGO_ENABLED=1 GOOS=linux go build -o docqa ./cmd/main.go

FROM debian:bookworm-slim

# 安装运行时依赖 - 将 libopenblas-base 改为 libopenblas0
RUN apt-get update && apt-get install -y \
    libgomp1 \
    libopenblas0 \
    ca-certificates \
    libstdc++6 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# 复制 FAISS 库和头文件
COPY --from=builder /usr/local/lib/ /usr/local/lib/
COPY --from=builder /usr/local/include/ /usr/local/include/
RUN ldconfig

WORKDIR /app

COPY --from=builder /app/docqa .

# 创建必要目录
RUN mkdir -p /app/data/files /app/data/vectordb /app/logs

ENV GIN_MODE=release

EXPOSE 8080

CMD ["/app/docqa"]