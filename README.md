# picoceph

Run Ceph and RADOS Gateway (RGW) in a single Docker container. Useful for developing and testing S3 applications.

## Usage

### Start

```shell
docker run --rm --name picoceph --privileged -v /dev:/dev -v /lib/modules:/lib/modules:ro -p7480:7480 -p8080:8080 ghcr.io/bucket-sailor/picoceph:latest
```

### S3

The RADOS Gateway S3 service is available at [http://localhost:7480](http://localhost:7480).

#### Create an S3 User

To create an admin user, run the following command:

```shell
docker exec -it picoceph radosgw-admin user create --uid="admin" --display-name="Admin User" --caps="users=*;buckets=*;metadata=*;usage=*;zone=*"
```

#### Create an S3 Access Key

To create a static key for the user, run the following command:

```shell
docker exec -it picoceph radosgw-admin key create --uid="admin" --key-type=s3 --access-key=admin --secret-key=admin
```

### Dashboard

The Ceph dashboard is available at [http://localhost:8080](http://localhost:8080).

#### Create a Dashboard User

To create an admin user, run the following command:

```shell
docker exec -it picoceph sh -c "echo 'p@ssw0rd' | ceph dashboard ac-user-create admin -i - administrator"
```