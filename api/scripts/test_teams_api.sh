#!/bin/bash

# Load environment variables
source .env.test

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo_step() {
    echo -e "${BLUE}=== $1 ===${NC}"
}

# 1. Create a test team
echo_step "1. Creating test team"
TEAM_RESPONSE=$(http POST $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN" \
    name="Test Team" \
    description="Test Description" \
    max_members:=10 \
    allow_member_invite:=true \
    default_member_role="member" \
    isolated_data:=true \
    shared_api_keys:=false)

TEAM_ID=$(echo $TEAM_RESPONSE | jq -r '.id')
echo -e "${GREEN}Created team with ID: $TEAM_ID${NC}"

# 2. List teams
echo_step "2. Listing teams"
echo "Basic listing:"
http GET $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN"

echo "With pagination:"
http GET $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN" \
    skip==0 limit==10

echo "With search:"
http GET $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN" \
    search=="Test Team"

# 3. Get team details
echo_step "3. Getting team details"
http GET $API_URL/v1/console/teams/$TEAM_ID \
    Authorization:"Bearer $TOKEN"

# 4. Update team
echo_step "4. Updating team"
echo "Updating basic info:"
http PUT $API_URL/v1/console/teams/$TEAM_ID \
    Authorization:"Bearer $TOKEN" \
    name="Updated Test Team" \
    description="Updated description"

echo "Updating settings:"
http PUT $API_URL/v1/console/teams/$TEAM_ID \
    Authorization:"Bearer $TOKEN" \
    max_members:=20 \
    allow_member_invite:=false

# 5. Member management
echo_step "5. Managing team members"

echo "Adding member:"
http POST $API_URL/v1/console/teams/$TEAM_ID/members \
    Authorization:"Bearer $TOKEN" \
    user_id:=123 \
    role="member"

echo "Listing members:"
http GET $API_URL/v1/console/teams/$TEAM_ID/members \
    Authorization:"Bearer $TOKEN"

echo "Updating member role:"
http PUT $API_URL/v1/console/teams/$TEAM_ID/members/123 \
    Authorization:"Bearer $TOKEN" \
    role="admin"

# 6. Invitation management
echo_step "6. Managing invitations"

echo "Creating invitation:"
INVITATION_RESPONSE=$(http POST $API_URL/v1/console/teams/$TEAM_ID/invitations \
    Authorization:"Bearer $TOKEN" \
    invitee_email="test@example.com" \
    role="member")

INVITATION_ID=$(echo $INVITATION_RESPONSE | jq -r '.id')

echo "Listing invitations:"
http GET $API_URL/v1/console/teams/$TEAM_ID/invitations \
    Authorization:"Bearer $TOKEN"

echo "Listing pending invitations:"
http GET $API_URL/v1/console/teams/$TEAM_ID/invitations \
    Authorization:"Bearer $TOKEN" \
    status=="pending"

echo "Cancelling invitation:"
http DELETE $API_URL/v1/console/teams/$TEAM_ID/invitations/$INVITATION_ID \
    Authorization:"Bearer $TOKEN"

# 7. Remove member
echo_step "7. Removing team member"
http DELETE $API_URL/v1/console/teams/$TEAM_ID/members/123 \
    Authorization:"Bearer $TOKEN"

# 8. Delete team
echo_step "8. Deleting team"
http DELETE $API_URL/v1/console/teams/$TEAM_ID \
    Authorization:"Bearer $TOKEN" \
    force==true

echo -e "${GREEN}All tests completed!${NC}"
