API for sending the username and other manual info to SQS

CRON to see if any data needs to be rescrapped and send to SQS

Receive SQS for processing the image downloading
    -> Calling api to scrape data
        (0.0027 per call or 0.0837 per detail call)
        (0.25 per call or 7 per detail call)
    -> Download all the images
    -> Send Raw for estimations (with Bias input which were sent manually)
    -> Translate the data in Socials Data
    -> Save translated data in mysql instantly





// -----------------

// func handler(ctx context.Context) (string, error) {

//  start := time.Now().UnixMicro()

//  log.Println("Lambda invocation start", start)

//  executeOnAll()

//  log.Println("Lambda invocation end", time.Now().UnixMicro())

//  return "ok", nil

// }

// func executeOnAll() {

//  startExecutionTime := time.Now().UnixMicro()

//  log.Println("Start Execution", startExecutionTime)

//  socials, err := trendlybq.SocialsN8N{}.GetPaginatedFromFirestore(0, 0)

//  if err != nil {

//      log.Println("Error ", err.Error())

//      return

//  }

//  for i, v := range socials {

//      socials[i] = *sui.MoveImagesToS3(&v)

//      socials[i].LastUpdateTime = time.Now().UnixMicro()

//      // Not saving to firestore as thats redundant. We anyway would be remiving all images url from the current export

//      // socials[i].InsertToFirestore()

//      log.Println("Done Social -", i, socials[i].LastUpdateTime, socials[i].ProfilePic)

//  }

//  log.Println("Total Socials", len(socials), startExecutionTime)

//  err = trendlybq.SocialsN8N{}.InsertMultiple(socials)

//  if err != nil {

//      log.Println("Error While Inserting", err.Error())

//      return

//  }

//  for _, v := range socials {

//      v.UpdateMinified()

//  }

//  log.Println("Done All")

// }
