PAT=EAANaW51FjsgBOwj50iKNnsgxtFn2eSC9jgwlMHHT2JtafZAaulo3sYi2u87t3Lm8riYGguwahnhp6HYIAZCO0I2pMK1p0ZBmRGnww2BpgZAzCXZCSWiIevsJUy5qM5z6Mhw5LBcLouLZCqv1dCvAjSBZA3eqd11eoWAPXZAEE59ByytvyuR1xFlHQJI7y6WQKN97fwZDZD
PAGEID=me
PLATFORM=instagram
access_token=EAAGDG5jzw5QBO57Niys7D5wRDd74zT4NEZAKZBnHAfuFW5W34nGZCQQRgIOX87VmtXS3XZC0wUxSp8by5TmtuTHjefXjEemS2nACMzy5GPofZAjOZC1AqSzcZB7I7ZAQn3ftj2mrBUXPzNLGCebZBxMZAKUTDZBEZA5j0ZAWYuyhZCAMgxwTXbnU58YqteQZBZBgCNZAD8rma1vqDhpUMDopTx7scnQZDZD
PAT2=EAANaW51FjsgBOwj50iKNnsgxtFn2eSC9jgwlMHHT2JtafZAaulo3sYi2u87t3Lm8riYGguwahnhp6HYIAZCO0I2pMK1p0ZBmRGnww2BpgZAzCXZCSWiIevsJUy5qM5z6Mhw5LBcLouLZCqv1dCvAjSBZA3eqd11eoWAPXZAEE59ByytvyuR1xFlHQJI7y6WQKN97fwZDZD

curl -i -X POST \
  "https://graph.facebook.com/v19.0/me/subscribed_apps?subscribed_fields=feed&access_token=EAAFB..."

# curl -i -X GET "https://graph.facebook.com/v19.0/oauth/access_token?grant_type=fb_exchange_token&client_id=425629530178452&client_secret=0babd776dab621585c2370dccec78f2f&fb_exchange_token=$access_token" 
# curl -i -X GET \
#  "https://graph.facebook.com/v19.0/me/accounts?fields=instagram_business_account&access_token=$access_token"
# curl -i -X GET "https://graph.facebook.com/v19.0/$PAGEID/conversations?platform=$PLATFORM&fields=name,id,messages\{to,message\}&access_token=$PAT"
# curl -i -X GET "https://graph.facebook.com/v19.0/aWdfZAG06MTpJR01lc3NhZA2VUaHJlYWQ6MTc4NDE0NjY2MTgxNTEyOTQ6MzQwMjgyMzY2ODQxNzEwMzAxMjQ0MjU5NTY1MTM4NTg2MTM3MTkx?fields=name,id,messages\{to,message\}&access_token=$PAT"

