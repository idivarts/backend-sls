# API to be called from Frontend

`<graph-api-url>`/me?fields=name,id,accounts{access_token,name,id,instagram_business_account}

# For Facebook

fields=id,name,about,category,category_list,location,phone,website,emails,fan_count,followers_count,picture{url},cover{source}

```json
{
  "id": "311133518746783",
  "name": "Trendly",
  "about": "Unveiling Trendly!🚀| One-stop solution to connect influencers with brands 🤝 | Elevate your collabs 📈📱 #staytuned",
  "category": "বিজ্ঞাপন/মার্কেটিং",
  "category_list": [
    {
      "id": "1757592557789532",
      "name": "বিজ্ঞাপন/মার্কেটিং"
    }
  ],
  "phone": "+919905264774",
  "website": "http://trendly.now/",
  "fan_count": 0,
  "followers_count": 0,
  "picture": {
    "data": {
      "url": "https://scontent.fccu4-3.fna.fbcdn.net/v/t39.30808-1/453500514_122134055294299159_3029197442927089117_n.jpg?stp=dst-jpg_s100x100_tt6&_nc_cat=106&ccb=1-7&_nc_sid=6738e8&_nc_ohc=PNMrX8WvDdgQ7kNvgFLrZsJ&_nc_zt=24&_nc_ht=scontent.fccu4-3.fna&edm=AJdBtusEAAAA&_nc_gid=AE-G9T7DjXhyPXLwAG6LkYM&oh=00_AYD5no3Y_0UiSd9YgoWDJThRl60XLzIamX1jwq4_3T8rHQ&oe=675F45D2"
    }
  },
  "cover": {
    "source": "https://scontent.fccu25-1.fna.fbcdn.net/v/t39.30808-6/464176619_122149503764299159_8025263945990025872_n.jpg?_nc_cat=105&ccb=1-7&_nc_sid=dc4938&_nc_ohc=OyGX-dAblycQ7kNvgFFZgJK&_nc_zt=23&_nc_ht=scontent.fccu25-1.fna&edm=AJdBtusEAAAA&_nc_gid=AE-G9T7DjXhyPXLwAG6LkYM&oh=00_AYDEXK1A4RwuqdEC4CQdEY96-jY2COUs2jSlqyEhgbiLWA&oe=675F3B25",
    "id": "122149503758299159"
  }
}
```

# For Instagram

fields=id,name,username,profile_picture_url,biography,followers_count,follows_count,media_count,website

```json
{
  "id": "17841466618151294",
  "name": "Trendly",
  "username": "trendly.now",
  "profile_picture_url": "https://scontent.fccu25-1.fna.fbcdn.net/v/t51.2885-15/452649260_1144764826622816_8824331081732466412_n.jpg?_nc_cat=107&ccb=1-7&_nc_sid=7d201b&_nc_ohc=Z2yAqGWBSqMQ7kNvgHXylAy&_nc_zt=23&_nc_ht=scontent.fccu25-1.fna&edm=AL-3X8kEAAAA&oh=00_AYDCg9vKtUGTldtD9BeeJ_cXeILqzZ1hOfutIe7UWVONjg&oe=675F3D44",
  "biography": "Unveiling Trendly!🚀| One-stop solution to connect influencers with brands 🤝 | Elevate your collabs 📈📱 #staytuned",
  "followers_count": 16,
  "follows_count": 120,
  "media_count": 22,
  "website": "https://trendly.now"
}
```

### Call the media api

If Media api return no data - then the account is private. Let the user know the same.

If it returns data, Let the user login


### Insights Details

https://developers.facebook.com/docs/instagram-platform/instagram-graph-api/reference/ig-user/insights

Refer this page
