// MongoDB script to add business management access role
// Run this in your MongoDB database

db.access_roles.insertOne({
  key: "business management",
  name: "Business Management"
});

print("Business management access role added successfully!"); 