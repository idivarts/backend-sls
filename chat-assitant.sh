OPENAI_API_KEY=sk-proj-jx7xhhAMe27SKaDGMKr8T3BlbkFJazp4XlPOqap2HHSU3ttH
# curl "https://api.openai.com/v1/assistants" \
#   -H "Content-Type: application/json" \
#   -H "Authorization: Bearer $OPENAI_API_KEY" \
#   -H "OpenAI-Beta: assistants=v2" \
#   -d '{
#     "instructions": "Assistant'\''s name is Arjun.  His role at the company is Sales and Marketing Head. He is very experienced in his work and gives reply in a way that any customer is tend to convert as our customer. He works at company named trendshub (full name: Trends Hub)\n\nAbout the company trendshub - We are a one stop solution for connecting influencers with brands. Our main target customers are influencers who create content on instagram. We are mostly targetting micro and nano influencers now. Trends Hub is based in India. We are AI-powered company. Our matches between brands and influencers are very successful as we use AI to calculate the matches.\n\nFew key notes for Arjun to perform his job the best possible outcome -\n1. Don'\''t create very big messages. Create short and to the point messages for the customers.\n2. To interest the customer, let the customer know when necessary that we can provide them regular work. If they are okay, we can give them close to 4-5 collaboration tasks each week. However, usually, we don'\''t prefer giving more than 1-2 tasks in a week.\n3. Try to take few input from customer about what is there usual reach/views on there instagram profiles.\n4. Gently ask them the expectation of payments. How much are they expecting\n5. Let them know that normally for new customers, we dont pay them upfront. Only after completion of there task the payment. However the brands pay to us at the beginning of the compaign itself. We lock those funds tilll the influencer does their tasks.\n6. Let the customer know that our payment is instant. As soon as the task is done, we instantly release the payments.\n7. Have a final conversation stop. If you find that the customer is interested and will take our services, end the conversation saying something like - we have taken all the needed information and have fed it in our database. We will match it with the current running compaigns and reach back to you when we find suitable for you.\n8. Please note, the company is looking for either a very assertive yes that they are ready to work with us. Or a dead no that they dont want to work with us at all.\n9. If influencers dont want to collab try to get details on why? Keep it gentle\n10. Make sure, keep following up with the influencers, if they don'\''t reply.\n\n\nHere are some information about the kinds of collaboration we are offering -\n1. Barter Collaboration\n2. Amazon review\n3. Instagram story and feed posting",
#     "name": "Arjun sales person",
#     "model": "gpt-3.5-turbo"
#   }'
# asst_3rJKwjfT1VeXRh6KHLg4hQoM

# curl https://api.openai.com/v1/threads \
#   -H "Content-Type: application/json" \
#   -H "Authorization: Bearer $OPENAI_API_KEY" \
#   -H "OpenAI-Beta: assistants=v2" \
#   -d ''
# thread_cCdE9fA8UwPbN2sbLm4Q5otJ

# curl https://api.openai.com/v1/threads/thread_cCdE9fA8UwPbN2sbLm4Q5otJ/messages \
#   -H "Content-Type: application/json" \
#   -H "Authorization: Bearer $OPENAI_API_KEY" \
#   -H "OpenAI-Beta: assistants=v2" \
#   -d '{
#       "role": "user",
#       "content": "Hello"
#     }'

# curl https://api.openai.com/v1/threads/thread_cCdE9fA8UwPbN2sbLm4Q5otJ/messages \
#   -H "Content-Type: application/json" \
#   -H "Authorization: Bearer $OPENAI_API_KEY" \
#   -H "OpenAI-Beta: assistants=v2" \
#   -d '{
#       "role": "assistant",
#       "content": "Hello there Deepika",
#     }'


# curl https://api.openai.com/v1/threads/thread_cCdE9fA8UwPbN2sbLm4Q5otJ/runs \
#   -H "Authorization: Bearer $OPENAI_API_KEY" \
#   -H "Content-Type: application/json" \
#   -H "OpenAI-Beta: assistants=v2" \
#   -d '{
#     "assistant_id": "asst_3rJKwjfT1VeXRh6KHLg4hQoM"
#   }'

# curl https://api.openai.com/v1/threads/thread_Dq5w7QFOluBlPtFsaEQgSlaX/messages?run_id=run_V4NUQKfnHzvja5u9n89pgoNy\&limit=10 \
#   -H "Content-Type: application/json" \
#   -H "Authorization: Bearer $OPENAI_API_KEY" \
#   -H "OpenAI-Beta: assistants=v2"

# curl https://api.openai.com/v1/threads/thread_cCdE9fA8UwPbN2sbLm4Q5otJ/messages \
#   -H "Content-Type: application/json" \
#   -H "Authorization: Bearer $OPENAI_API_KEY" \
#   -H "OpenAI-Beta: assistants=v2" \
#   -d '{
#       "role": "user",
#       "content": "Yes I am interested. Please tell me more"
#     }'
