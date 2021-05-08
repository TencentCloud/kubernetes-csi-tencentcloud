#!/bin/sh
# If you omit that part, the command will be run as root.
exec /bin/csi-chdfs --drivername=com.tencent.cloud.csi.chdfs --nodeid=$NODE_ID --endpoint=$CSI_ENDPOINT --logtostderr=true  -v=5  2>&1

