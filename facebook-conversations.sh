PAT=EAANaW51FjsgBOwj50iKNnsgxtFn2eSC9jgwlMHHT2JtafZAaulo3sYi2u87t3Lm8riYGguwahnhp6HYIAZCO0I2pMK1p0ZBmRGnww2BpgZAzCXZCSWiIevsJUy5qM5z6Mhw5LBcLouLZCqv1dCvAjSBZA3eqd11eoWAPXZAEE59ByytvyuR1xFlHQJI7y6WQKN97fwZDZD
PAGEID=me
PLATFORM=instagram
curl -i -X GET "https://graph.facebook.com/v19.0/$PAGEID/conversations?platform=$PLATFORM&fields=name,id,messages\{to,message\}&access_token=$PAT"
# curl -i -X GET "https://graph.facebook.com/v19.0/aWdfZAG06MTpJR01lc3NhZA2VUaHJlYWQ6MTc4NDE0NjY2MTgxNTEyOTQ6MzQwMjgyMzY2ODQxNzEwMzAxMjQ0MjU5NTY1MTM4NTg2MTM3MTkx?fields=name,id,messages\{to,message\}&access_token=$PAT"

