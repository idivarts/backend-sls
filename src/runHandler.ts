import { SQSEvent, SQSHandler } from "aws-lambda";
import OpenAI from 'openai';

const openai = new OpenAI({
    apiKey: process.env['OPENAI_API_KEY'], // This is the default and can be omitted
});

export const runHandler: SQSHandler = async (event: SQSEvent) => {
    // openai.beta.threads.runs.stream
    for (const record of event.Records) {
        const { body } = record;
        console.log(`Processing SQS message: ${body}`);

        // Do your processing logic here (e.g., parsing the message, processing, etc.)
        try {
            // Simulate message processing
            const message = JSON.parse(body);
            console.log("Message content:", message);

            // You can add more processing logic here

        } catch (error) {
            console.error("Error processing SQS message:", error);
            throw new Error("Message processing failed");
        }
    }
};