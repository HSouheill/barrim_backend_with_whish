# Notification System Implementation

This document describes the Firebase Cloud Messaging (FCM) notification system implemented in the Barrim backend.

## Overview

The notification system allows sending push notifications to service providers and users through Firebase Cloud Messaging. It includes endpoints for:

1. Sending notifications to service providers
2. Updating FCM tokens for users and service providers

## API Endpoints

### 1. Send Notification to Service Provider

**Endpoint:** `POST /api/notifications/send-to-service-provider`

**Description:** Sends a push notification to a specific service provider.

**Request Body:**
```json
{
  "serviceProviderId": "string (required)",
  "title": "string (required)",
  "message": "string (required)",
  "data": {
    "type": "booking_request",
    "bookingId": "string",
    "customerName": "string",
    "serviceType": "string",
    "bookingDate": "string",
    "timeSlot": "string",
    "isEmergency": "boolean"
  }
}
```

**Response:**
```json
{
  "success": true,
  "message": "Notification sent successfully",
  "messageId": "projects/barrim-93482/messages/..."
}
```

### 2. Update Service Provider FCM Token

**Endpoint:** `POST /api/service-provider/fcm-token`

**Description:** Updates the FCM token for the authenticated service provider.

**Authentication:** Required (JWT token)

**Request Body:**
```json
{
  "fcmToken": "string (required)"
}
```

**Response:**
```json
{
  "success": true,
  "message": "FCM token updated"
}
```

### 3. Update User FCM Token

**Endpoint:** `POST /api/users/fcm-token`

**Description:** Updates the FCM token for the authenticated user.

**Authentication:** Required (JWT token)

**Request Body:**
```json
{
  "fcmToken": "string (required)"
}
```

**Response:**
```json
{
  "success": true,
  "message": "FCM token updated"
}
```

## Database Schema Changes

### User Model
Added `fcmToken` field:
```go
FCMToken string `json:"fcmToken,omitempty" bson:"fcmToken,omitempty"`
```

### ServiceProvider Model
Added `fcmToken` field:
```go
FCMToken string `json:"fcmToken,omitempty" bson:"fcmToken,omitempty"`
```

## Firebase Configuration

The system uses Firebase Admin SDK for Go. Make sure you have:

1. Firebase service account key file (`barrim-3b45a-firebase-adminsdk-fbsvc-44cc12116d.json`)
2. Firebase project ID: `barrim-93482`
3. Firebase initialized in `config/firebase.go`

## Notification Features

### Android Configuration
- Priority: High
- Sound: Default
- Channel ID: `barrim_fcm_channel`

### iOS Configuration
- Sound: Default
- Badge: 1
- Category: `BOOKING_REQUEST`

## Usage Examples

### Sending a Booking Request Notification

```bash
curl -X POST http://localhost:8080/api/notifications/send-to-service-provider \
  -H "Content-Type: application/json" \
  -d '{
    "serviceProviderId": "60f7b3b3b3b3b3b3b3b3b3b3",
    "title": "New Booking Request",
    "message": "You have a new booking request from John Doe",
    "data": {
      "type": "booking_request",
      "bookingId": "60f7b3b3b3b3b3b3b3b3b3b4",
      "customerName": "John Doe",
      "serviceType": "Plumbing",
      "bookingDate": "2024-01-15",
      "timeSlot": "10:00 AM",
      "isEmergency": "false"
    }
  }'
```

### Updating FCM Token

```bash
curl -X POST http://localhost:8080/api/service-provider/fcm-token \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "fcmToken": "dGVzdF9mY21fdG9rZW5fZXhhbXBsZQ=="
  }'
```

## Error Handling

The system handles various error scenarios:

- Invalid service provider ID
- Service provider not found
- Missing FCM token
- Firebase messaging errors
- Database connection errors

All errors return appropriate HTTP status codes and error messages.

## Security Considerations

1. FCM token update endpoints require authentication
2. Service provider ID validation prevents unauthorized access
3. Input validation on all request bodies
4. Proper error handling without exposing sensitive information

## Testing

To test the notification system:

1. Ensure Firebase is properly configured
2. Create a test service provider with a valid FCM token
3. Use the send notification endpoint to test message delivery
4. Verify FCM token updates work correctly

## Dependencies

The implementation uses the following Go packages:

- `firebase.google.com/go/v4/messaging` - Firebase Admin SDK
- `github.com/labstack/echo/v4` - Web framework
- `go.mongodb.org/mongo-driver` - MongoDB driver
