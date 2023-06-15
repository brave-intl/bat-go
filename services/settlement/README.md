## Settlement Automation

Settlement automation uses Redis streams for messaging and implements a message abstraction over the underlying infrastructure (see below). There are six consumers representing the different stages of the settlement process, the first stage is prepare which places transactions in the prepare state and begins the automated flow through each stage.   

When a transaction is added to the prepare stream a message route is attached based on the transaction type for example, ads or grants. At each stage if the message is processed successfully it will be advanced to the next stage in the route otherwise sent to deadletter queue if it reaches a retry limit for that route. 
