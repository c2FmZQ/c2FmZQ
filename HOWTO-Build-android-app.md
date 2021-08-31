# Build Stingle Photos for self hosted server

Using the [thyrlian/AndroidSDK](https://github.com/thyrlian/AndroidSDK) docker
image to build the Stingle Photos app is straight forward.

```
# Pull the image.
docker pull thyrlian/android-sdk

# Create a working directory.
mkdir -p ~/android && cd ~/android

# Initialize the build environment.
docker run -it --rm -v $(pwd)/sdk:/sdk thyrlian/android-sdk bash -c 'cp -a $ANDROID_SDK_ROOT/. /sdk'

# Pull the source code.
git clone https://github.com/c2FmZQ/stingle-photos-for-self-hosted-server.git

# Enter the build environment.
docker run -it --rm -v $(pwd)/sdk:/opt/android-sdk -v $(pwd)/gradle:/root/.gradle -v $(pwd)/stingle-photos-for-self-hosted-server:/src thyrlian/android-sdk /bin/bash
```

Now, inside the build environment:

```
# Go the source directory.
cd /src

# Create a dummy keystore.
keytool -v -dname "CN=Unknown, OU=Unknown, O=Unknown, L=Unknown, ST=Unknown, C=Unknown" -genkeypair -storepass test123 -keypass test123 -alias main -keyalg RSA -keysize 2048 -validity 10000 -keystore /src/keystore

# with a matching keystore.properties.
echo "storePassword=test123" > keystore.properties
echo "keyPassword=test123" >> keystore.properties
echo "keyAlias=main" >> keystore.properties
echo "storeFile=/src/keystore" >> keystore.properties

# Build and try to install the APK.
./gradlew installFdroidRelease

# Install will fail if there is no android device connected, but the APK should
# still be there.
ls -l ./StinglePhotos/build/outputs/apk/fdroid/release/StinglePhotos-fdroid-release.apk
```

