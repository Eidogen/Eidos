#!/bin/bash
#
# Eidos Proto Generation Script
# Generates Go code from all Protocol Buffer definitions
#
# Usage: ./scripts/gen_proto.sh
#
# Requirements:
#   - protoc (Protocol Buffers compiler)
#   - protoc-gen-go (Go protobuf plugin)
#   - protoc-gen-go-grpc (Go gRPC plugin)
#
# Install dependencies:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get the project root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
PROTO_DIR="$PROJECT_ROOT/proto"

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Eidos Proto Generation Script${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check for required tools
check_tool() {
    if ! command -v $1 &> /dev/null; then
        echo -e "${RED}Error: $1 is not installed${NC}"
        echo "Please install it first:"
        echo "  $2"
        exit 1
    fi
}

echo -e "${YELLOW}Checking required tools...${NC}"
check_tool "protoc" "brew install protobuf (macOS) or apt-get install protobuf-compiler (Linux)"
check_tool "protoc-gen-go" "go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
check_tool "protoc-gen-go-grpc" "go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
echo -e "${GREEN}All required tools are installed${NC}"
echo ""

# Create output directories if they don't exist
create_output_dirs() {
    local dirs=(
        "$PROTO_DIR/common"
        "$PROTO_DIR/trading/v1"
        "$PROTO_DIR/matching/v1"
        "$PROTO_DIR/market/v1"
        "$PROTO_DIR/chain/v1"
        "$PROTO_DIR/risk/v1"
        "$PROTO_DIR/admin/v1"
        "$PROTO_DIR/jobs/v1"
        "$PROTO_DIR/settlement/v1"
    )

    for dir in "${dirs[@]}"; do
        mkdir -p "$dir"
    done
}

# Generate proto files for a specific package
generate_proto() {
    local proto_file=$1
    local proto_name=$(basename "$proto_file")
    local proto_dir=$(dirname "$proto_file")

    echo -e "${YELLOW}Generating: ${proto_name}${NC}"

    protoc \
        --proto_path="$PROTO_DIR" \
        --go_out="$PROTO_DIR" \
        --go_opt=paths=source_relative \
        --go-grpc_out="$PROTO_DIR" \
        --go-grpc_opt=paths=source_relative \
        "$proto_file"

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}  Generated successfully${NC}"
    else
        echo -e "${RED}  Failed to generate${NC}"
        exit 1
    fi
}

# Clean old generated files
clean_generated() {
    echo -e "${YELLOW}Cleaning old generated files...${NC}"
    find "$PROTO_DIR" -name "*.pb.go" -type f -delete
    find "$PROTO_DIR" -name "*_grpc.pb.go" -type f -delete
    echo -e "${GREEN}Cleaned${NC}"
    echo ""
}

# Main generation process
main() {
    cd "$PROJECT_ROOT"

    # Clean old files
    clean_generated

    # Create output directories
    create_output_dirs

    echo -e "${YELLOW}Generating Protocol Buffer files...${NC}"
    echo ""

    # Generate common proto files first (dependencies)
    echo -e "${GREEN}[1/8] Generating common protos...${NC}"
    for proto_file in "$PROTO_DIR"/common/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    # Generate trading service protos
    echo -e "${GREEN}[2/8] Generating trading service protos...${NC}"
    for proto_file in "$PROTO_DIR"/trading/v1/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    # Generate matching service protos
    echo -e "${GREEN}[3/8] Generating matching service protos...${NC}"
    for proto_file in "$PROTO_DIR"/matching/v1/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    # Generate market service protos
    echo -e "${GREEN}[4/8] Generating market service protos...${NC}"
    for proto_file in "$PROTO_DIR"/market/v1/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    # Generate chain service protos
    echo -e "${GREEN}[5/8] Generating chain service protos...${NC}"
    for proto_file in "$PROTO_DIR"/chain/v1/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    # Generate risk service protos
    echo -e "${GREEN}[6/8] Generating risk service protos...${NC}"
    for proto_file in "$PROTO_DIR"/risk/v1/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    # Generate admin service protos
    echo -e "${GREEN}[7/8] Generating admin service protos...${NC}"
    for proto_file in "$PROTO_DIR"/admin/v1/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    # Generate jobs service protos
    echo -e "${GREEN}[8/8] Generating jobs service protos...${NC}"
    for proto_file in "$PROTO_DIR"/jobs/v1/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    # Generate settlement service protos
    echo -e "${GREEN}[Bonus] Generating settlement service protos...${NC}"
    for proto_file in "$PROTO_DIR"/settlement/v1/*.proto; do
        if [ -f "$proto_file" ]; then
            generate_proto "$proto_file"
        fi
    done
    echo ""

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  Proto generation completed!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    # List generated files
    echo -e "${YELLOW}Generated files:${NC}"
    find "$PROTO_DIR" -name "*.pb.go" -type f | sort
}

# Run main function
main "$@"
