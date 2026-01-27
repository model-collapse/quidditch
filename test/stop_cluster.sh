#!/bin/bash
#
# Stop Quidditch Cluster
#

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

DATA_DIR="${DATA_DIR:-/tmp/quidditch-test}"

echo "=========================================="
echo " Stopping Quidditch Cluster"
echo "=========================================="
echo ""

# Function to stop a process
stop_process() {
    local name=$1
    local pid_file=$2

    if [ -f "$pid_file" ]; then
        PID=$(cat "$pid_file")
        if ps -p $PID > /dev/null 2>&1; then
            echo "Stopping $name (PID: $PID)..."
            kill $PID
            sleep 2
            if ps -p $PID > /dev/null 2>&1; then
                echo "  Force killing..."
                kill -9 $PID
            fi
            echo -e "${GREEN}✓ $name stopped${NC}"
        else
            echo "  $name not running"
        fi
        rm -f "$pid_file"
    else
        echo "  No PID file for $name"
    fi
}

# Stop nodes
stop_process "Coordination Node" "$DATA_DIR/coordination.pid"
stop_process "Data Node" "$DATA_DIR/data.pid"
stop_process "Master Node" "$DATA_DIR/master.pid"

echo ""
echo -e "${GREEN}✓ Cluster stopped${NC}"
echo ""

exit 0
