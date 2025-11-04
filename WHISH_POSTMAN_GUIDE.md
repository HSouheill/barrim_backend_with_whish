# Testing Whish API with Postman

This guide shows you how to test the Whish payment API integration using Postman.

## Prerequisites

1. Postman installed on your computer
2. Backend server running on `http://localhost:8080`
3. Environment variables configured in your `.env` file:
   ```env
   WHISH_ENVIRONMENT=testing
   WHISH_CHANNEL=your_test_channel
   WHISH_SECRET=your_test_secret
   WHISH_WEBSITE_URL=https://your-website.com
   ```

## Import Postman Collection

You can manually create the following requests or import the collection provided below.

### Base URL
```
http://localhost:8080/api/whish
```

---

## 1. Get Balance

**Request Type:** GET

**Endpoint:** 
```
GET http://localhost:8080/api/whish/balance
```

**Headers:** None required (handled by backend)

**Body:** None

**Example Response:**
```json
{
  "status": true,
  "balance": 217.718
}
```

---

## 2. Get Rate (Get Current Payment Fees)

**Request Type:** POST

**Endpoint:**
```
POST http://localhost:8080/api/whish/rate
```

**Headers:**
```
Content-Type: application/json
```

**Body (raw JSON):**
```json
{
  "amount": 100.0,
  "currency": "LBP"
}
```

**Expected Response:**
```json
{
  "status": true,
  "rate": 0.041666666666666667
}
```

**Alternative Examples:**
```json
// USD currency
{
  "amount": 50.0,
  "currency": "USD"
}

// LBP with different amount
{
  "amount": 250.0,
  "currency": "LBP"
}
```

---

## 3. Create Payment (Post Payment)

**Request Type:** POST

**Endpoint:**
```
POST http://localhost:8080/api/whish/payment
```

**Headers:**
```
Content-Type: application/json
```

**Body (raw JSON):**
```json
{
  "amount": 100.0,
  "currency": "LBP",
  "invoice": "Payment for Test Order #123",
  "externalId": 12345,
  "successCallbackUrl": "https://your-app.com/success",
  "failureCallbackUrl": "https://your-app.com/failure",
  "successRedirectUrl": "https://your-app.com/payment-success",
  "failureRedirectUrl": "https://your-app.com/payment-failed"
}
```

**Expected Response:**
```json
{
  "status": true,
  "collectUrl": "https://whish.money/pay/8nQS2mL"
}
```

**Alternative Examples:**

For AED currency:
```json
{
  "amount": 50.0,
  "currency": "AED",
  "invoice": "Payment for Service XYZ",
  "externalId": 54321,
  "successCallbackUrl": "https://your-app.com/success",
  "failureCallbackUrl": "https://your-app.com/failure",
  "successRedirectUrl": "https://your-app.com/payment-success",
  "failureRedirectUrl": "https://your-app.com/payment-failed"
}
```

---

## 4. Get Payment Status

**Request Type:** POST

**Endpoint:**
```
POST http://localhost:8080/api/whish/payment/status
```

**Headers:**
```
Content-Type: application/json
```

**Body (raw JSON):**
```json
{
  "currency": "LBP",
  "externalId": 12345
}
```

**Expected Response:**
```json
{
  "status": true,
  "collectStatus": "success",
  "payerPhoneNumber": "96170902894"
}
```

**Possible Status Values:**
- `"success"` - Payment completed successfully
- `"failed"` - Payment failed
- `"pending"` - Payment is pending

---

## Step-by-Step Postman Setup

### Option 1: Manual Setup

1. **Open Postman**
2. **Create a new request**
3. **Select the request type** (GET or POST)
4. **Enter the URL**: `http://localhost:8080/api/whish/[endpoint]`
5. **For POST requests:**
   - Go to "Body" tab
   - Select "raw"
   - Choose "JSON" from the dropdown
   - Paste the request body JSON
6. **Click "Send"**

### Option 2: Using Environment Variables (Recommended)

1. **Create a Postman Environment:**
   - Click the gear icon (top right)
   - Click "Add" to create a new environment
   - Add variables:
     - `base_url`: `http://localhost:8080`
     - `whish_path`: `api/whish`

2. **Use variables in URLs:**
   ```
   {{base_url}}/{{whish_path}}/balance
   {{base_url}}/{{whish_path}}/rate
   {{base_url}}/{{whish_path}}/payment
   {{base_url}}/{{whish_path}}/payment/status
   ```

---

## Testing Workflow

### Complete Payment Flow Test:

1. **Get Balance** - Check your initial balance
   ```
   GET {{base_url}}/{{whish_path}}/balance
   ```

2. **Get Rate** - Check the fee rate for 100 LBP payment
   ```
   POST {{base_url}}/{{whish_path}}/rate
   Body: {"amount": 100.0, "currency": "LBP"}
   ```

3. **Create Payment** - Initiate a payment
   ```
   POST {{base_url}}/{{whish_path}}/payment
   Body: {
     "amount": 100.0,
     "currency": "LBP",
     "invoice": "Test Payment",
     "externalId": 99999,
     "successCallbackUrl": "https://your-app.com/success",
     "failureCallbackUrl": "https://your-app.com/failure",
     "successRedirectUrl": "https://your-app.com/success",
     "failureRedirectUrl": "https://your-app.com/failure"
   }
   ```
   
   **Copy the `collectUrl` from the response**

4. **Test the Payment Page** - Open the `collectUrl` in a browser
   - For testing: Use phone `96170902894` with OTP `111111`
   - Complete the payment

5. **Check Payment Status**
   ```
   POST {{base_url}}/{{whish_path}}/payment/status
   Body: {
     "currency": "LBP",
     "externalId": 99999
   }
   ```

6. **Verify Balance** - Check balance after payment
   ```
   GET {{base_url}}/{{whish_path}}/balance
   ```

---

## Important Notes

### Sandbox Testing Credentials:
- **Success Test:**
  - Phone: `96170902894`
  - OTP: `111111`
  
- **Failure Test:**
  - Use any other phone number
  - Use any OTP other than `111111`

### Currencies Supported:
- `LBP` (Lebanese Pound)
- `USD` (US Dollar)
- `AED` (UAE Dirham)

### Common Errors:

1. **Missing environment variables:**
   ```
   Error: environment variable not set
   ```
   **Solution:** Check your `.env` file has all required variables

2. **Invalid credentials:**
   ```
   Error: whish API error: unauthorized
   ```
   **Solution:** Verify your `WHISH_CHANNEL` and `WHISH_SECRET` are correct

3. **Connection error:**
   ```
   Error: failed to send request
   ```
   **Solution:** Ensure your backend server is running

---

## Postman Collection JSON (Import This)

Save this as `Whish_API.postman_collection.json` and import into Postman:

```json
{
  "info": {
    "name": "Whish API",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "item": [
    {
      "name": "Get Balance",
      "request": {
        "method": "GET",
        "header": [],
        "url": {
          "raw": "http://localhost:8080/api/whish/balance",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["api", "whish", "balance"]
        }
      }
    },
    {
      "name": "Get Rate",
      "request": {
        "method": "POST",
        "header": [
          {
            "key": "Content-Type",
            "value": "application/json"
          }
        ],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"amount\": 100.0,\n  \"currency\": \"LBP\"\n}"
        },
        "url": {
          "raw": "http://localhost:8080/api/whish/rate",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["api", "whish", "rate"]
        }
      }
    },
    {
      "name": "Create Payment",
      "request": {
        "method": "POST",
        "header": [
          {
            "key": "Content-Type",
            "value": "application/json"
          }
        ],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"amount\": 100.0,\n  \"currency\": \"LBP\",\n  \"invoice\": \"Test Payment\",\n  \"externalId\": 12345,\n  \"successCallbackUrl\": \"https://your-app.com/success\",\n  \"failureCallbackUrl\": \"https://your-app.com/failure\",\n  \"successRedirectUrl\": \"https://your-app.com/payment-success\",\n  \"failureRedirectUrl\": \"https://your-app.com/payment-failed\"\n}"
        },
        "url": {
          "raw": "http://localhost:8080/api/whish/payment",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["api", "whish", "payment"]
        }
      }
    },
    {
      "name": "Get Payment Status",
      "request": {
        "method": "POST",
        "header": [
          {
            "key": "Content-Type",
            "value": "application/json"
          }
        ],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"currency\": \"LBP\",\n  \"externalId\": 12345\n}"
        },
        "url": {
          "raw": "http://localhost:8080/api/whish/payment/status",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["api", "whish", "payment", "status"]
        }
      }
    }
  ]
}
```

---

## Quick Reference Card

| Endpoint | Method | URL |
|----------|--------|-----|
| Get Balance | GET | `/api/whish/balance` |
| Get Rate | POST | `/api/whish/rate` |
| Create Payment | POST | `/api/whish/payment` |
| Get Status | POST | `/api/whish/payment/status` |

Now you're ready to test the Whish API integration! 🚀

