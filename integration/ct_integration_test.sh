#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

RPC_PORT=36962
CT_PORT=6962

# Build config file with absolute paths
CT_CFG=$(mktemp ${INTEGRATION_DIR}/ct-XXXXXX)
sed "s!@TESTDATA@!${TESTDATA}!" ./integration/ct_integration_test.cfg > ${CT_CFG}
trap "rm ${CT_CFG}" EXIT

# Retrieve tree IDs from config file
TREE_IDS=$(grep LogID ${CT_CFG} | grep -o '[0-9]\+'| xargs)
for id in ${TREE_IDS}
do
    echo "Provisioning test log (Tree ID: ${id}) in database"
    ${SCRIPTS_DIR}/wipelog.sh ${id}
    ${SCRIPTS_DIR}/createlog.sh ${id}
done

echo "Starting Log RPC server on port ${RPC_PORT}"
pushd ${TRILLIAN_ROOT} > /dev/null
go build ${GOFLAGS} ./server/trillian_log_server/
./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${RPC_PORT} --signer_interval="1s" --sequencer_sleep_between_runs="1s" --batch_size=100 &
RPC_SERVER_PID=$!
popd > /dev/null

# Set an exit trap to ensure we kill the RPC server once we're done.
trap "kill -INT ${RPC_SERVER_PID}" EXIT
waitForServerStartup ${RPC_PORT}

echo "Starting CT HTTP server on port ${CT_PORT}"
pushd ${TRILLIAN_ROOT} > /dev/null
go build ${GOFLAGS} ./examples/ct/ct_server/
./ct_server --log_config=${CT_CFG} --log_rpc_server="localhost:${RPC_PORT}" --port=${CT_PORT} &
HTTP_SERVER_PID=$!
popd > /dev/null

# Set an exit trap to ensure we kill the servers once we're done.
trap "kill -INT ${HTTP_SERVER_PID} ${RPC_SERVER_PID}" EXIT
# The server will 404 the request as there's no handler for it. This error doesn't matter
# as the test will fail if the server is really not up.
set +e
waitForServerStartup ${CT_PORT}
set -e

echo "Running test(s)"
set +e
go test -v -tags=integration -run ".*CT.*" --timeout=5m ./integration --log_config ${CT_CFG} --ct_http_server="localhost:${CT_PORT}" --testdata=${TESTDATA}
RESULT=$?
set -e

rm ${CT_CFG}
trap - EXIT
echo "Stopping CT HTTP server (pid ${HTTP_SERVER_PID}) on port ${CT_PORT}"
kill -INT ${HTTP_SERVER_PID}

echo "Stopping Log RPC server (pid ${RPC_SERVER_PID}) on port ${RPC_PORT}"
kill -INT ${RPC_SERVER_PID}

if [ $RESULT != 0 ]; then
    sleep 1
    if [ "$TMPDIR" == "" ]; then
        TMPDIR=/tmp
    fi
    echo "RPC Server log:"
    echo "--------------------"
    cat ${TMPDIR}/trillian_log_server.INFO
    echo "HTTP Server log:"
    echo "--------------------"
    cat ${TMPDIR}/ct_server.INFO
    exit $RESULT
fi
