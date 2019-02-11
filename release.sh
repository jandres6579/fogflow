# build the images of all FogFlow core components
./build.sh

if [ $# -gt 0 ]; then
    VERSION=$1
    echo "releasing v${VERSION} to docker hub"
    
    # rename images from latest to the specific version
    docker image tag fogflow/discovery:latest fogflow/discovery:${VERSION}
    docker image tag fogflow/broker:latest fogflow/broker:${VERSION}
    docker image tag fogflow/master:latest fogflow/master:${VERSION}
    docker image tag fogflow/worker:latest fogflow/worker:${VERSION}

    # publish images to the docker hub
    docker publish fogflow/discovery:${VERSION}
    docker publish fogflow/broker:${VERSION}
    docker publish fogflow/master:${VERSION}
    docker publish fogflow/worker:${VERSION}
fi 



