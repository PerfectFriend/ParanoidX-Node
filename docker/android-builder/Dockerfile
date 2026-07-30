# Android APK Builder for The Isle project
# Build: docker build -t isle-android-builder .
# Run:   docker run --rm -v /path/to/project:/app isle-android-builder

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV FLUTTER_HOME=/opt/flutter
ENV ANDROID_HOME=/opt/android-sdk
ENV PATH=$FLUTTER_HOME/bin:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$ANDROID_HOME/build-tools/34.0.0:$PATH
ENV PUB_HOSTED_URL=https://pub.dev
ENV FLUTTER_STORAGE_BASE_URL=https://storage.googleapis.com

# Install dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl git unzip xz-utils zip libglu1-mesa clang cmake \
    ninja-build pkg-config libgtk-3-dev liblzma-dev \
    openjdk-17-jdk-headless \
    && rm -rf /var/lib/apt/lists/*

# Install Flutter
RUN git clone --depth 1 -b stable https://github.com/flutter/flutter.git $FLUTTER_HOME \
    && flutter precache --android \
    && flutter config --android-sdk $ANDROID_HOME \
    && dart --version && flutter --version

# Install Android SDK command-line tools
RUN mkdir -p $ANDROID_HOME/cmdline-tools \
    && curl -fsSL https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip -o /tmp/cmdline-tools.zip \
    && unzip -q /tmp/cmdline-tools.zip -d $ANDROID_HOME/cmdline-tools \
    && mv $ANDROID_HOME/cmdline-tools/cmdline-tools $ANDROID_HOME/cmdline-tools/latest \
    && rm /tmp/cmdline-tools.zip

# Accept licenses and install SDK components
RUN yes | sdkmanager --licenses > /dev/null 2>&1 || true \
    && sdkmanager "platform-tools" "platforms;android-34" "build-tools;34.0.0" \
    "ndk;25.2.9519653" "cmake;3.22.1" > /dev/null 2>&1

WORKDIR /app

# Copy project files (shared by volume mount at runtime)
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
