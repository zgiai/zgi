# Console Teams API Testing Guide

## Prerequisites
```bash
# Install HTTPie
brew install httpie

# Set environment variables
export API_URL=http://localhost:7001
export TOKEN=your_access_token
```

## 1. Team Listing

### List all teams
```bash
# Basic listing
http GET $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN"

# With pagination
http GET $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN" \
    skip==0 limit==10

# With search
http GET $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN" \
    search=="test team"
```

## 2. Team Details

### Get team details
```bash
# Get specific team
http GET $API_URL/v1/console/teams/1 \
    Authorization:"Bearer $TOKEN"
```

## 3. Team Updates

### Update team information
```bash
# Update basic info
http PUT $API_URL/v1/console/teams/1 \
    Authorization:"Bearer $TOKEN" \
    name="Updated Team Name" \
    description="Updated description"

# Update settings
http PUT $API_URL/v1/console/teams/1 \
    Authorization:"Bearer $TOKEN" \
    max_members:=20 \
    allow_member_invite:=true \
    isolated_data:=false

# Note: Use := for boolean and number values
```

## 4. Team Deletion

### Delete team
```bash
# Normal deletion
http DELETE $API_URL/v1/console/teams/1 \
    Authorization:"Bearer $TOKEN"

# Force deletion (when team has members)
http DELETE $API_URL/v1/console/teams/1 \
    Authorization:"Bearer $TOKEN" \
    force==true
```

## 5. Member Management

### List team members
```bash
# List members with pagination
http GET $API_URL/v1/console/teams/1/members \
    Authorization:"Bearer $TOKEN" \
    skip==0 limit==10
```

### Add team member
```bash
# Add member with role
http POST $API_URL/v1/console/teams/1/members \
    Authorization:"Bearer $TOKEN" \
    user_id:=123 \
    role="admin"
```

### Update member role
```bash
# Update member's role
http PUT $API_URL/v1/console/teams/1/members/123 \
    Authorization:"Bearer $TOKEN" \
    role="member"
```

### Remove team member
```bash
# Remove member
http DELETE $API_URL/v1/console/teams/1/members/123 \
    Authorization:"Bearer $TOKEN"
```

## 6. Invitation Management

### List invitations
```bash
# List all invitations
http GET $API_URL/v1/console/teams/1/invitations \
    Authorization:"Bearer $TOKEN"

# List with status filter
http GET $API_URL/v1/console/teams/1/invitations \
    Authorization:"Bearer $TOKEN" \
    status=="pending"
```

### Create invitation
```bash
# Create new invitation
http POST $API_URL/v1/console/teams/1/invitations \
    Authorization:"Bearer $TOKEN" \
    invitee_email="user@example.com" \
    role="member"
```

### Cancel invitation
```bash
# Cancel existing invitation
http DELETE $API_URL/v1/console/teams/1/invitations/1 \
    Authorization:"Bearer $TOKEN"
```

## Testing Script
```bash
#!/bin/bash

# Set variables
API_URL="http://localhost:7001"
TOKEN="your_access_token"

# 1. Create a test team
echo "Creating test team..."
TEAM_ID=$(http POST $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN" \
    name="Test Team" \
    description="Test Description" \
    max_members:=10 | jq -r '.id')

# 2. List teams
echo "Listing teams..."
http GET $API_URL/v1/console/teams \
    Authorization:"Bearer $TOKEN"

# 3. Get team details
echo "Getting team details..."
http GET $API_URL/v1/console/teams/$TEAM_ID \
    Authorization:"Bearer $TOKEN"

# 4. Update team
echo "Updating team..."
http PUT $API_URL/v1/console/teams/$TEAM_ID \
    Authorization:"Bearer $TOKEN" \
    name="Updated Test Team"

# 5. Add member
echo "Adding team member..."
http POST $API_URL/v1/console/teams/$TEAM_ID/members \
    Authorization:"Bearer $TOKEN" \
    user_id:=123 \
    role="member"

# 6. Create invitation
echo "Creating invitation..."
http POST $API_URL/v1/console/teams/$TEAM_ID/invitations \
    Authorization:"Bearer $TOKEN" \
    invitee_email="test@example.com" \
    role="member"

# 7. List members
echo "Listing team members..."
http GET $API_URL/v1/console/teams/$TEAM_ID/members \
    Authorization:"Bearer $TOKEN"

# 8. List invitations
echo "Listing invitations..."
http GET $API_URL/v1/console/teams/$TEAM_ID/invitations \
    Authorization:"Bearer $TOKEN"

# 9. Clean up
echo "Cleaning up..."
http DELETE $API_URL/v1/console/teams/$TEAM_ID \
    Authorization:"Bearer $TOKEN" \
    force==true
```

## Common Response Codes

- 200: Success
- 201: Created
- 400: Bad Request
- 401: Unauthorized
- 403: Forbidden
- 404: Not Found
- 422: Validation Error
- 500: Server Error

## Tips

1. Use `jq` for JSON processing:
```bash
# Get team ID from response
TEAM_ID=$(http POST $API_URL/v1/console/teams ... | jq -r '.id')
```

2. Use environment variables:
```bash
# Set in your .env or .bashrc
export API_URL=http://localhost:7001
export TOKEN=your_access_token
```

3. For bulk operations, use loops:
```bash
# Create multiple teams
for i in {1..5}; do
    http POST $API_URL/v1/console/teams \
        Authorization:"Bearer $TOKEN" \
        name="Team $i"
done
```
