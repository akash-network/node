pragma solidity ^0.4.18;


contract Maintainable {
    address public maintainer;
    function Maintainable(address rootCaller) public {
      maintainer = rootCaller;
    }
    modifier onlyMaintainer() {
        require(msg.sender == maintainer);
        _;
    }
    function transferMaintainorship(address newMaintainer) public onlyMaintainer {
        require(newMaintainer != address(0));
        maintainer = newMaintainer;
    }
}

contract Ownable {
    address public owner;
    function Ownable() public {
        owner = msg.sender;
    }
    modifier onlyOwner() {
        require(msg.sender == owner);
        _;
    }
}

contract BadActor {
    bool public badActor;
    function BadActor() public {
        badActor = false;
    }
    modifier notBadActor() {
        require(!badActor);
        _;
    }
    modifier isBadActor() {
        require(badActor);
        _;
    }
}

contract Payable is BadActor {
    function () public notBadActor payable {}
}

contract Matchable is Payable {
    bool public matched;
    function Matchable() public {
        matched = false;
    }
    modifier notMatched() {
        require(!matched);
        _;
    }
    modifier isMatched() {
        require(matched);
        _;
    }
}

contract Parameterized is Maintainable, Matchable {

    uint public ram;
    uint public cpu;
    uint public rate;
    uint public minimumBalanace;
    uint public cancelFee;

    function Parameterized(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee) public {
        ram = _ram;
        cpu = _cpu;
        rate = _rate;
        minimumBalanace = _minimumBalanace;
        cancelFee = _cancelFee;
    }
    function modify(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee) public onlyMaintainer {
        ram = _ram;
        cpu = _cpu;
        rate = _rate;
        minimumBalanace = _minimumBalanace;
        cancelFee = _cancelFee;
    }
}

 contract Client is Ownable, Parameterized {

    Provider public matchedProvider;
    uint public matchStartTime;
    uint public totalBilled;
    string public manifest;

    // Constructor
    function Client(address maintainer, uint ram, uint cpu, uint rate, uint _minimumBalanace, uint cancelFee, string _manifest)
        public payable Parameterized(ram, cpu, rate, _minimumBalanace, cancelFee) Maintainable(maintainer)
    {
        manifest = _manifest;
    }

    // match with provider
    function matchProvider(address providerAddress) external onlyOwner notMatched notBadActor returns (bool) {
        matchedProvider = Provider(providerAddress);
        // set parameters to that of the provider
        ram = matchedProvider.ram();
        cpu = matchedProvider.cpu();
        rate = matchedProvider.rate();
        minimumBalanace = matchedProvider.minimumBalanace();
        cancelFee= matchedProvider.cancelFee();
        matchStartTime = now;
        matched = true;
        return matched;
    }

    // send the value of the outstanding bill to the provider
    function bill() public isMatched {
         uint payment;
        // pay required amount since last payment time
        uint unsettledBalance = (now - matchStartTime) * rate - totalBilled;
        // pay the provider
        if (this.balance > unsettledBalance) {
            payment = unsettledBalance;
        } else {
            payment = this.balance;
        }
        // update the total amount billed and unsetted
        totalBilled += payment;
        // send payment
        matchedProvider.transfer(payment);

        // if new balance is less than minimumBalanace, set as badActor
        if (this.balance < minimumBalanace) {
            badActor = true;
        }
    }

    // allow maintainer to withdrawal ETH
    function withdrawal(uint amount) public onlyMaintainer notBadActor {
        // ensure contract balance does not fall below minimumBalanace
        if (matched) {
            amount = amount - minimumBalanace;
        }
        maintainer.transfer(amount);
    }

    // reset contract fields
    function reset() private {
        matched = false;
        matchedProvider = Provider(0x0);
    }

    // reset contract fields. Prevents payments.
    function cancel() public onlyMaintainer isMatched {
        bill();
        matchedProvider.clientCancel();
        reset();
    }

    // lets provider cancel the contract
    function providerCancel() external {
        bill();
        // only matchedProvider should be able to call this
        require(msg.sender == address(matchedProvider));
        if (badActor) {
             matchedProvider.transfer(this.balance);
        }
        reset();
    }
}

contract Provider is Ownable, Parameterized {

    Client public matchedClient;
    string public networkAddress;

    // Constructor
    function Provider(address maintainer, uint ram, uint cpu, uint rate, uint minimumBalanace, uint cancelFee, string _networkAddress)
        public Parameterized(ram, cpu, rate, minimumBalanace, cancelFee) Maintainable(maintainer)
    {
        networkAddress = _networkAddress;
    }

    // confirm and declare that a valid matching contract is at an address
    function matchClient(address clientAddress) external onlyOwner notMatched notBadActor returns (bool) {
        // load client contract
        Client client = Client(clientAddress);
        // make sure resources match
        require(client.cpu() <= cpu
        && client.ram() <= ram
        && client.rate() >= rate
        && client.minimumBalanace() >= minimumBalanace
        && client.cancelFee() <= cancelFee);
        matchedClient = client;
        matched = true;
        return matched;
    }

    // reset contract fields
    function reset() private {
        matched = false;
        matchedClient = Client(0x0);
    }

    // maintainer cancels contract and can incur a fee
    function cancel() public onlyMaintainer isMatched  {
        // cause the client to cancel
        matchedClient.providerCancel();
        // if the matchedClient is not delinquent send an early cancellation fee
        if (!matchedClient.badActor()) {
            matchedClient.transfer(cancelFee);
        }
        reset();
    }

    // client cancels contract
    function clientCancel() external isMatched {
        // only matchedClient should be able to call this
        require(msg.sender == address(matchedClient));
        if (badActor) {
            matchedClient.transfer(this.balance);
        }
        reset();
    }

    function makeBadActor() external notBadActor onlyOwner {
        badActor = true;
    }

    // send maximum allowable funds to contract maintainer
    function withdrawal() public notBadActor {
         uint amount = this.balance;
        if (matched) {
            // ensure contract always can pay the early cancel fee when not canceled
            amount = amount - cancelFee;
        }
        require(amount > 0);
        maintainer.transfer(amount);
    }
}

contract Master is Maintainable {

    function Master() public Maintainable(msg.sender) {}
    mapping(address => uint) public badActors;

    // call to put an ask for a service on the network
    function deployClient(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee, string _manifest)
        public payable returns (Client)
    {
        require(msg.value >= _minimumBalanace);
        Client client = new Client(msg.sender, _ram, _cpu, _rate, _minimumBalanace, _cancelFee, _manifest);
        client.transfer(msg.value);
        return client;
    }

    // call to put a bid for a service on the network
    function deployProvider(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee, string _networkAddress)
        public payable returns (Provider)
    {
        require(msg.value >= _cancelFee);
        Provider provider = new Provider(msg.sender, _ram, _cpu, _rate, _minimumBalanace, _cancelFee, _networkAddress);
        provider.transfer(msg.value);
        return provider;
    }

    // match a Provider and Client
    function matchContracts(address clientAddress, address providerAddress) onlyMaintainer public {
        Provider provider = Provider(providerAddress);
        Client client = Client(clientAddress);
        // check if provider can match with client and  match provider with client
        require(provider.matchClient(clientAddress));
        // match client with provider
        client.matchProvider(providerAddress);
    }

    // mark a Provider contract as a bad actor and record the address
    // off-chain maintain a databse of bad actor contract networkAddress
    function makeBadActor(address providerAddress) onlyMaintainer public {
        Provider provider = Provider(providerAddress);
        provider.makeBadActor();
        badActors[providerAddress] += 1;
    }
}
