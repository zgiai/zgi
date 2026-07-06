#!/bin/bash

# Setup Billing Proto Files
# This script copies billing.proto from console-api and generates Go code

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color
PROTOC_GEN_GO_VERSION="v1.36.10"
PROTOC_GEN_GO_GRPC_VERSION="v1.6.1"

echo -e "${GREEN}=== ZGI API Billing Proto Setup ===${NC}\n"

# Check if console-api path is provided
CONSOLE_API_PATH="${1:-../zgi-console-api}"

if [ ! -d "$CONSOLE_API_PATH" ]; then
    echo -e "${RED}Error: Console API path not found: $CONSOLE_API_PATH${NC}"
    echo "Usage: $0 [path-to-console-api]"
    echo "Example: $0 ../zgi-console-api"
    exit 1
fi

# Check if billing.proto exists in console-api
PROTO_SOURCE="$CONSOLE_API_PATH/pkg/rpc/v1/billing.proto"
if [ ! -f "$PROTO_SOURCE" ]; then
    echo -e "${RED}Error: billing.proto not found at: $PROTO_SOURCE${NC}"
    exit 1
fi

echo -e "${YELLOW}Step 1: Creating directories...${NC}"
mkdir -p pkg/rpc/v1
echo -e "${GREEN}✓ Created pkg/rpc/v1${NC}\n"

echo -e "${YELLOW}Step 2: Copying billing.proto...${NC}"
cp "$PROTO_SOURCE" pkg/rpc/v1/billing.proto
echo -e "${GREEN}✓ Copied billing.proto from console-api${NC}\n"

echo -e "${YELLOW}Step 3: Checking protoc installation...${NC}"
if ! command -v protoc &> /dev/null; then
    echo -e "${RED}Error: protoc is not installed${NC}"
    echo "Install with:"
    echo "  macOS: brew install protobuf"
    echo "  Linux: apt-get install protobuf-compiler"
    exit 1
fi
echo -e "${GREEN}✓ protoc is installed: $(protoc --version)${NC}\n"

echo -e "${YELLOW}Step 4: Checking protoc-gen-go plugins...${NC}"
if ! command -v protoc-gen-go &> /dev/null; then
    echo -e "${YELLOW}Installing protoc-gen-go...${NC}"
    go install google.golang.org/protobuf/cmd/protoc-gen-go@"$PROTOC_GEN_GO_VERSION"
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo -e "${YELLOW}Installing protoc-gen-go-grpc...${NC}"
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@"$PROTOC_GEN_GO_GRPC_VERSION"
fi
echo -e "${GREEN}✓ protoc plugins are installed${NC}\n"

echo -e "${YELLOW}Step 5: Generating Go code from proto...${NC}"
protoc --proto_path=. \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  pkg/rpc/v1/billing.proto

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Generated billing.pb.go${NC}"
    echo -e "${GREEN}✓ Generated billing_grpc.pb.go${NC}\n"
else
    echo -e "${RED}Error: Failed to generate Go code${NC}"
    exit 1
fi

echo -e "${YELLOW}Step 6: Verifying generated files...${NC}"
if [ -f "pkg/rpc/v1/billing.pb.go" ] && [ -f "pkg/rpc/v1/billing_grpc.pb.go" ]; then
    echo -e "${GREEN}✓ All files generated successfully${NC}\n"
else
    echo -e "${RED}Error: Generated files not found${NC}"
    exit 1
fi

echo -e "${GREEN}=== Setup Complete ===${NC}\n"
echo "Generated files:"
echo "  - pkg/rpc/v1/billing.proto"
echo "  - pkg/rpc/v1/billing.pb.go"
echo "  - pkg/rpc/v1/billing_grpc.pb.go"
echo ""
echo "Next steps:"
echo "  1. Update internal/infra/platform/billing/remote.go with new methods"
echo "  2. Test compilation: go build ./internal/infra/platform/billing/..."
echo "  3. Test standalone mode: go run cmd/server/main.go"
echo "  4. Test cloud mode: ZGI_RUN_MODE=cloud go run cmd/server/main.go"
echo ""
echo "See docs/PLATFORM_BILLING_INTEGRATION.md for details"
