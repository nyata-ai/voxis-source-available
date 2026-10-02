ARG DEBIAN_BUILD_IMAGE=debian@sha256:6ebd97fa83deb272194a2cf015b3d26a4d538e9ad3a7a79d544c8af5b0a01443
ARG DEBIAN_RUNTIME_IMAGE=debian@sha256:d7e12182ce18b85b93007c1dedf31f2d29e01ccf3182cc4017c709b6259bc132

FROM ${DEBIAN_BUILD_IMAGE} AS build
ARG LLAMA_CPP_REVISION
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates cmake g++ git libcurl4-openssl-dev libssl-dev make \
    && rm -rf /var/lib/apt/lists/*
RUN test -n "$LLAMA_CPP_REVISION" \
    && git clone https://github.com/ggml-org/llama.cpp.git /src/llama.cpp \
    && git -C /src/llama.cpp checkout --detach "$LLAMA_CPP_REVISION" \
    && test "$(git -C /src/llama.cpp rev-parse HEAD)" = "$LLAMA_CPP_REVISION"
RUN cmake -S /src/llama.cpp -B /src/llama.cpp/build -DGGML_NATIVE=OFF -DGGML_CURL=ON -DLLAMA_OPENSSL=ON -DLLAMA_BUILD_UI=OFF -DLLAMA_USE_PREBUILT_UI=OFF \
    && cmake --build /src/llama.cpp/build --parallel 8 --target llama-server

FROM ${DEBIAN_RUNTIME_IMAGE}
RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install -y --no-install-recommends ca-certificates curl libcurl4 libgomp1 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 --shell /usr/sbin/nologin llama
COPY --from=build /src/llama.cpp/build/bin/llama-server /usr/local/bin/llama-server
COPY --from=build /src/llama.cpp/build/bin/*.so /usr/local/lib/
COPY --from=build /src/llama.cpp/LICENSE /usr/share/licenses/llama.cpp/LICENSE
RUN ldconfig
USER llama
ENV HOME=/home/llama XDG_CACHE_HOME=/home/llama/.cache
ENTRYPOINT ["/usr/local/bin/llama-server"]
