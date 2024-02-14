# picoceph

Run Ceph and RADDS Gateway (RGW) in a single Docker container. Useful for testing S3 applications.

## Usage

### Start Container

```shell
docker run --rm --name picoceph --privileged -v /dev:/dev -v /lib/modules:/lib/modules:ro -p7480:7480 ghcr.io/bucket-sailor/picoceph:latest
```

The S3 service is available at [http://localhost:7480](http://localhost:7480).

### Create User

To create an admin user, run the following command:

```shell
docker exec -it picoceph radosgw-admin user create --uid="admin" --display-name="Admin User" --caps="users=*;buckets=*;metadata=*;usage=*;zone=*"
```

### Add a Key

To create a static key for the user, run the following command:

```shell
docker exec -it picoceph radosgw-admin key create --uid="admin" --key-type=s3 --access-key=admin --secret-key=admin
```