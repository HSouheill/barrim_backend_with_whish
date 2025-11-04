# Mobile App FCM Setup Guide

## 🚨 Important: The system does NOT automatically detect if a device belongs to a service provider
The mobile app MUST send the FCM token to the backend when the service provider logs in.

## Setup Instructions for Mobile App

### ⚠️ IMPORTANT: Send FCM Token in Multiple Scenarios

The FCM token should be sent to the backend in these situations:

1. **When user logs in** (after successful authentication)
2. **When app starts** (if user is already logged in - stored JWT)
3. **When FCM token is refreshed** (Firebase SDK may refresh tokens periodically)

This ensures notifications work even when the app is closed or the device restarts.

### API Endpoints to Use

#### For Service Providers:
```http
POST https://barrim.online/api/service-provider/fcm-token
Authorization: Bearer {JWT_TOKEN}
Content-Type: application/json

{
  "fcmToken": "YOUR_FCM_TOKEN_HERE"
}
```

#### For Regular Users:
```http
POST https://barrim.online/api/users/fcm-token
Authorization: Bearer {JWT_TOKEN}
Content-Type: application/json

{
  "fcmToken": "YOUR_FCM_TOKEN_HERE"
}
```

## Example Implementation

### 🔑 Key Concept: Background Notifications

**Will notifications work when the app is closed?**
- ✅ **YES** - If the FCM token is properly stored and sent
- ✅ Android: Shows notification in system tray even when app is closed
- ✅ iOS: Shows notification in system tray even when app is closed
- ⚠️ **Requires**: FCM token to be sent to backend when app starts

**Important**: Since your app keeps users logged in (persistent JWT), you MUST also send the FCM token when the app starts, not just when they explicitly log in.

### iOS (Swift):
```swift
import FirebaseMessaging

// IMPORTANT: Call this in multiple places:
// 1. After successful login
// 2. When app starts (if user is already logged in)
// 3. When FCM token is refreshed

// In AppDelegate.swift or SceneDelegate.swift
func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
    // Check if user is already logged in
    if isUserLoggedIn() {
        sendFCMTokenToServer()
    }
    return true
}

func sendFCMTokenToServer() {
    Messaging.messaging().token { token, error in
        if let error = error {
            print("Error getting FCM token: \(error)")
        } else if let token = token {
            // Send token to backend
            sendTokenToBackend(token: token)
        }
    }
}

// Also listen for token refresh
override func tokenRefresh(_ tokenRefreshNotification: Notification) {
    Messaging.messaging().token { token, error in
        if let error = error {
            print("Error getting FCM token: \(error)")
        } else if let token = token {
            print("FCM token refreshed: \(token)")
            sendTokenToBackend(token: token)
        }
    }
}

func sendTokenToBackend(token: String) {
    guard let jwtToken = getJWTToken() else { return }
    
    let url = URL(string: "https://barrim.online/api/service-provider/fcm-token")!
    var request = URLRequest(url: url)
    request.httpMethod = "POST"
    request.setValue("Bearer \(jwtToken)", forHTTPHeaderField: "Authorization")
    request.setValue("application/json", forHTTPHeaderField: "Content-Type")
    
    let body = ["fcmToken": token]
    request.httpBody = try? JSONSerialization.data(withJSONObject: body)
    
    URLSession.shared.dataTask(with: request) { data, response, error in
        if let error = error {
            print("Error sending FCM token: \(error)")
        } else {
            print("FCM token sent successfully")
        }
    }.resume()
}
```

### Android (Kotlin):
```kotlin
import com.google.firebase.messaging.FirebaseMessaging

// IMPORTANT: Call this in multiple places:
// 1. After successful login
// 2. When app starts (if user is already logged in)
// 3. When FCM token is refreshed

// In Application class or MainActivity onCreate
class MyApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        
        // Check if user is already logged in
        if (isUserLoggedIn()) {
            sendFCMTokenToServer()
        }
    }
}

// In your activity
fun sendFCMTokenToServer() {
    FirebaseMessaging.getInstance().token.addOnCompleteListener { task ->
        if (!task.isSuccessful) {
            Log.w(TAG, "Fetching FCM registration token failed", task.exception)
            return@addOnCompleteListener
        }
        
        val token = task.result
        Log.d(TAG, "FCM token: $token")
        
        // Send token to backend
        sendTokenToBackend(token)
    }
}

// Listen for token refresh
class MyFirebaseMessagingService : FirebaseMessagingService() {
    override fun onNewToken(token: String) {
        super.onNewToken(token)
        Log.d(TAG, "FCM token refreshed: $token")
        sendTokenToBackend(token)
    }
}

fun sendTokenToBackend(token: String) {
    val jwtToken = getJWTToken() ?: return
    
    val url = "https://barrim.online/api/service-provider/fcm-token"
    val request = JSONObject().apply {
        put("fcmToken", token)
    }
    
    // Make HTTP POST request with JWT token in Authorization header
    // ... your HTTP client code here
}
```

## 🔧 CRITICAL: FCM Token Refresh on App Start

Since your JWT tokens don't expire, the app needs to update the FCM token every time it starts, not just on login.

### The Problem:
```
1. User logs in → FCM token sent to backend ✅
2. User closes app
3. User reopens app → JWT is still valid (from storage)
4. App thinks user is logged in → But FCM token is NOT sent ❌
5. Backend has old/invalid FCM token
6. Notifications fail ❌
```

### The Solution:

**Send FCM token on EVERY app start if user is logged in**

#### iOS Example:
```swift
// In AppDelegate.swift
func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
    
    // Check if user has stored JWT token
    if let jwtToken = getStoredJWTToken(), !jwtToken.isEmpty {
        // User is logged in, update FCM token
        updateFCMTokenOnAppStart()
    }
    
    return true
}

func updateFCMTokenOnAppStart() {
    Messaging.messaging().token { token, error in
        if let token = token {
            sendFCMTokenToBackend(token: token, userType: getUserType())
        }
    }
}
```

#### Android Example:
```kotlin
// In MainActivity or Application class
override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    
    // Check if user has stored JWT token
    if (hasValidJWTToken()) {
        // User is logged in, update FCM token
        updateFCMTokenOnAppStart()
    }
}

fun updateFCMTokenOnAppStart() {
    FirebaseMessaging.getInstance().token.addOnCompleteListener { task ->
        if (task.isSuccessful) {
            sendFCMTokenToBackend(task.result, getUserType())
        }
    }
}
```

## Testing the Setup

### 1. Check if FCM Token is Stored
Query your database to verify:
```javascript
// Check service provider has FCM token
db.serviceProviders.findOne({ _id: ObjectId("YOUR_SERVICE_PROVIDER_ID") }, { fcmToken: 1 })

// Check user has FCM token
db.users.findOne({ _id: ObjectId("YOUR_USER_ID") }, { fcmToken: 1 })
```

### 2. Test Notification
When a booking is created, the service provider should receive a push notification on their device.

### 3. Troubleshooting

#### No notification received when app is closed?
1. **Check FCM token is stored** - Query the database
   ```bash
   db.serviceProviders.findOne({ _id: ObjectId("...") }, { fcmToken: 1 })
   ```
2. **Check if token was sent on app start** - Since you keep users logged in, the token must be sent every time the app starts
3. **Check Firebase credentials** - Make sure Firebase Admin SDK is properly configured
4. **Check device is online** - FCM requires internet connection
5. **Check device logs** - Look for Firebase errors
6. **Test notification** - Use Postman to send a test notification:
   ```bash
   curl -X POST "https://barrim.online/api/notifications/send-to-service-provider" \
     -H "Content-Type: application/json" \
     -d '{
       "serviceProviderId": "68f8991743d7e235e1646a79",
       "title": "Test",
       "message": "Testing notification"
     }'
   ```

#### Common Issues:
- **App doesn't send token on startup**: Add FCM token sending in `applicationDidFinishLaunching` (iOS) or `onCreate` (Android)
- **Token expired**: FCM tokens refresh periodically, implement `onNewToken` handler
- **Notification payload**: Current setup uses proper Notification + Data payload which works in all states

#### FCM token update fails?
1. **Check JWT token** - Make sure it's valid and not expired
2. **Check user is service provider** - Verify user type in JWT
3. **Check API endpoint** - Use correct endpoint for user type

## Important Notes

1. **FCM tokens expire** - The app should periodically refresh and update the token
2. **Multiple devices** - Currently, the system stores one token per user (last device wins)
3. **Token refresh** - If you want to support multiple devices, you'll need to modify the backend to store an array of tokens
4. **Privacy** - FCM tokens can be used to send notifications but don't reveal personal information

## Next Steps for Multi-Device Support

If you want service providers to receive notifications on multiple devices, modify the backend to:
1. Store FCM tokens as an array: `fcmTokens: [string]`
2. Send notifications to all tokens in the array
3. Handle token refresh and cleanup of invalid tokens
