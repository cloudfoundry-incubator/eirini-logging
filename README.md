# eirini-logging

[Eirinix](https://github.com/SUSE/eirinix) extension for Application Logging in Cloud Foundry

**Note** This is a work in progress to replace fluentd in Eirini

## Build

    $> make build

You can also build the Docker image that is used by the injected sidecar container:

    $> DOCKER_ORG="test/" make image

Or define the result image name:

    $> DOCKER_IMAGE="eirini-sidecar" make image

And later on consume it when running the extension

    $> DOCKER_SIDECAR_IMAGE="eirini-sidecar" ./binaries/eirini-logging


## Run

The extension is listening by default to 10,0.2.2, you can tweak that by setting  ```HOST```. 
You can also set a listening port specifying the environment variable ```PORT``` , and the namespace with ```NAMESPACE```.

Refer to the [sample extension](https://github.com/SUSE/eirinix-sample#eirinix-sample) for how to run this