Velero plugin for WebDAV
========================

## Overview

This plugin adds support for using a WebDAV server as object store to velero.

## Compatibility

The following table lists which plugin versions is compatible with which velero version.

| Plugin Version | Velero Version |
| -------------- | -------------- |
| v1.0.x         | v1.13.x        |

## Setup

To use this plugin you need to have a WebDAV server. This plugin can be configured by setting the following properties of the backup storage location:

| Property         | Required | Description                                                                           |
| ---------------- | -------- | ------------------------------------------------------------------------------------- |
| `root`           | ✅       | WebDAV server URL                                                                     |
| `user`           | ✅       | WebDAV username                                                                       |
| `webDAVPassword` | ❌       | password for the WebDAV server; defaults to empty password                            |
| `bucketsDir`     | ❌       | top level directory where the backups are stored on the WebDAV server, separate subdirectories with `/`, i. e. `mybuckets/clusterbackups` |
| `bucket`         | ✅       | name of the bucket (i. e. subdirectory in `bucketsDir`)                               |
| `delimiter`      | ❌       | object store delimiter; defaults to `/`, support for other delimiters is experimental |
| `logLevel`       | ❌       | set the log level of the plugin, valid values are `WARN`, `INFO` and `DEBUG`          |

Note that the plugin will not immediately create any directories or files in the WebDAV storage. These are created once they are needed.

You can find docker images for this plugin [here](https://hub.docker.com/repository/docker/talinx/velero-plugin-for-webdav).
