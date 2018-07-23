# The Akash Testnet

### About the testnet
The Akash testnet is a fully-functioning decentralized cloud, with support for requesting, depploying, and paying for cloud deployments. Server capacity is being kindly provided by Packet, the world's leading bare-metal provider. Access is **free** for registered users. As a free service, capacity is tightly managed, so please treat testnet capacity as the scarce community resource it is.  In other words, please please play nicely in our sandbox.


We want your feedback!  Please message us [on Telegram](https://t.me/AkashNW) with any and all of your feedback.  We're putting our in-progess platform out there so that we can get real-world feedback, so don't be shy!


Finally, some warnings. The Akash testnet is at an alpha-level stage of development and so **not intended for production use.**  New functionality and capacity is being added constantly, but is always presented to you as-is, so use at your own risk. Use is at our discretion and we reserve the right to bring down deployments at any time for any reason.

### Intended users
Fundamentally, the Akash testnet is a deployment platform with a CLI and intended for relatively sophisticated users.  If you are comfortable managing instances via the AWS API and can build and deploy a Docker container, you will find the Akash testnet easy to use.  If not, please feel free to give it a shot, but you might find it confusing.

### Getting help
First of course, run `akash -h` and RTFM, then please feel free to ask questions in our Telegram channel https://t.me/AkashNW.


## Getting started
The testnet supports this basic workflow:
 1. Register and receive testnet tokens
 1. Deploy your image
 1. Close your deployment (i.e. deprovision containers)

The sections below describe each step.


### Register for the testnet
You must register with Akash to request access to the testnet. This is a one-time only action.  After registering, you will receive a set of Akash testnet tokens **Please note that testnet tokens are only usable on the Akash testnet and have no market value.**


To register:
   1. Go to https://akash.network/testnet/ and follow the instructions.
   1. You'll immediately receive a confirmation email.  Click the link inside to confirm your email and request testnet tokens.
   1. After review by our staff, we will send tokens to your address and confirm with another email.

If you wish to receive another token allocation, repeat these steps and we will happily consider your request


### Deploy your image
In this step, you actually use the testnet to deploy an image, paying with your testnet tokens.

#### 1. Check your balance
```
$ akash key list #returns your key names and values
$ akash query account [key value] #returns your balance
```
For example:
```
$ akash key list
my-key-name 4b5446b97930b1885d11550cb2b277b6fee8e3ce

$ akash query account 4b5446b97930b1885d11550cb2b277b6fee8e3ce
{
  "address": "4b5446b97930b1885d11550cb2b277b6fee8e3ce",
  "balance": 420,
  "nonce": 1
}
```
#### 2. Download the sample deployment file and modify it if desired. 
The sample deployment file specifies a small webapp container running a simple demo site we created.  [You may download it here](testnet-deployment.yml) and use it as-is or modify it if you wish.


Feel free to modify the deployment file according to our [SDL (Stack Definition Language) documentation](../sdl.md). A typical modification would be to reference your own image instead of our demo app image.  Note that you are limited in the amount of testnet resources you may request. Please see the [Limitations section](#testnet-limitations).

#### 3. Create a new deployment
In this step, you post your deployment, the Akash marketplace matches you with a provider via auction, and your image is deployed.
 ```
 $ akash key list
 $ akash deploy <deployment file path> -k <key name> #creates and sends the deployment
 ```
 The CLI client will print the deployment address, bid and lease data to console, for example:
 ```
$ akash deployment create ./testnet-deployment.yml -k my-key-name
66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357
Waiting...
Group 1/1 Fulfillment: 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357/1/2/49877504638723665f08dd57c2b0fbae79bd2abf65fe0d397e20880953b9befc [price=20]
Group 1/1 Fulfillment: 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357/1/2/a8954503bdd62134bf691c954d4eba3099952424ed708c7b69afeecaa8f9b38f [price=44]
Group 1/1 Lease: 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357/1/2/49877504638723665f08dd57c2b0fbae79bd2abf65fe0d397e20880953b9befc [price=20]
```
Where the lease is in the form [deployment address]/[deployment group number]/[order number]/[provider address]. You may also query your leases with `akash query lease`. For example:
```
$ akash query lease
{
  "items": [
    {
      "id": {
        "deployment": "66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357",
        "group": 1,
        "order": 2,
        "provider": "49877504638723665f08dd57c2b0fbae79bd2abf65fe0d397e20880953b9befc"
      },
      "price": 20
    }
  ]
}

```


#### 4.  Access your deployed application in whatever way makes sense to you
You may also view your application logs with `akash logs <service name> <lease>`. For example, given a service named `webapp`:

```
$ ./akash logs webapp 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357/1/2/49877504638723665f08dd57c2b0fbae79bd2abf65fe0d397e20880953b9befc -l 1 -f
[web-58c8984dfd-nbf6g]  2018-07-04T01:57:55.141522165Z 172.17.0.5 - - [04/Jul/2018:01:57:55 +0000] "GET /images/favicon.png HTTP/1.1" 200 1825 "http://hello.192.168.99.100.nip.io/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.99 Safari/537.36" "192.168.99.1"
[web-58c8984dfd-nbf6g]  2018-07-04T01:57:57.255819449Z 172.17.0.5 - - [04/Jul/2018:01:57:57 +0000] "GET /images/favicon.png HTTP/1.1" 200 1825 "http://hello.192.168.99.100.nip.io/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.99 Safari/537.36" "192.168.99.1"
[web-58c8984dfd-nbf6g]  2018-07-04T02:03:04.221319604Z 172.17.0.5 - - [04/Jul/2018:02:03:04 +0000] "GET / HTTP/1.1" 304 0 "-" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.99 Safari/537.36" "192.168.99.1"
```

### Close your deployment
When you are done with your application, close the deployment. This will deprovision your container and stop the token transfer. This is a critical step to conserve both your tokens and testnet server capacity.
```
$ akash deployment close <deployment address> -k <key name>
```

For example:
```
$ akash deployment close 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357 -k my-key-name
Closing deployment
```


### Testnet Limitations

#### Resoource consumption
you are limited in such and such a way - a deployment that exceeds those limits will...


#### Supported regions
These regions are currently supported by the testnet. More will come onine, so check back frequently.
 - sjc
 - nrt









