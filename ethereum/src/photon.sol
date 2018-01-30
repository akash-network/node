pragma solidity ^0.4.18;

contract BadActor {
    bool public badActor;
    function BadActor() public {
        badActor = false;
    }
    modifier notBadActor() {
        require(!badActor);
        _;
    }
}

contract Matchable {
    bool public matched;
    function Matchable() public {
        matched = false;
    }
    modifier notMatched() {
        require(!matched);
        _;
    }
}

contract Cancelable {
    bool public canceled;
    modifier isCanceled() {
        require(canceled);
        _;
    }
    modifier notCanceled() {
        require(!canceled);
        _;
    }
}

contract Payable {
    function () public payable {}
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

contract Delinquent {
    bool public delinquent;
    function Delinquent() public {
        delinquent = false;
    }
    modifier isDelinquent() {
        require(delinquent);
        _;
    }
    modifier notDelinquent() {
        require(!delinquent);
        _;
    }
}

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
        if (newMaintainer != address(0)) {
          maintainer = newMaintainer;
        }
    }
}

contract Parameterized is Maintainable {

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

contract Provider is Ownable, Parameterized, Matchable, Cancelable, Payable, BadActor {

    // address of matched client
    Client public matchedClient;
    // Provider's public endpoint for Manifest distribution
    string public networkAddress;

    // Constructor
    function Provider(address maintainer, uint ram, uint cpu, uint rate, uint minimumBalanace, uint cancelFee, string _networkAddress) public Parameterized(ram, cpu, rate, minimumBalanace, cancelFee) Maintainable(maintainer) {
        networkAddress = _networkAddress;
    }

    // confirm and declare that a valid matching contract is at an address
    function matchClient(address clientAddress) public onlyOwner notCanceled notMatched returns (bool) {
        // load client contract
        Client client = Client(clientAddress);
        // make sure resources match
        require(client.cpu() <= cpu && client.ram() <= ram && client.rate() >= rate && client.minimumBalanace() >= minimumBalanace && client.cancelFee() <= cancelFee);
        matchedClient = client;
        matched = true;
        return matched;
    }

    // reset contract fields. Charge fee if early cancelation. Sends contract balance to maintainer
    function cancel() public notCanceled onlyMaintainer {
        matched = false;
        canceled = true;

        // if the matchedClients balance has falled below the minium balance do not charge an early cancellation fee
        if (matchedClient.balance >= matchedClient.minimumBalanace()) {
            matchedClient.transfer(cancelFee);
        }
        maintainer.transfer(this.balance);
    }

    // makes canceled false. contract can be rematched to a provider.
    function uncancel() public isCanceled {
        canceled = false;
    }

    // send funds to contract maintainer
    function withdrawal() public {
        uint amount = this.balance - cancelFee;
        // ensure contract always can pay the early cancel fee
        require(amount > 0);
        maintainer.transfer(amount);
    }
}

 contract Client is Ownable, Parameterized, Matchable, Cancelable, Payable, Delinquent {

    Provider public matchedProvider;
    uint public matchStartTime;
    uint public totalBilled;
    uint public unsettledBalance;
    uint public maxUnsettledBalance;

    // Constructor
    function Client(address maintainer, uint ram, uint cpu, uint rate, uint _minimumBalanace, uint cancelFee, uint _maxUnsettledBalance) public payable Parameterized(ram, cpu, rate, _minimumBalanace, cancelFee) Maintainable(maintainer) {
        maxUnsettledBalance = _maxUnsettledBalance;
    }

    // match with provider
    function matchProvider(address providerAddress) public onlyOwner notCanceled notMatched returns (bool) {
        matchedProvider = Provider(providerAddress);
        matchStartTime = now;
        matched = true;
        return matched;
    }

    // create a bill for the client.
    function setBill() public {
        // pay required amount since last payment time
        // get the required payment by total payment - expected payment
        uint expectedBilled = (now - matchStartTime) * rate;
        unsettledBalance = expectedBilled - totalBilled;
    }

    // send the value of the outstanding bill to the provider
    function bill() public notCanceled {
        // pay the provider
        uint payment;
        if (this.balance > unsettledBalance) {
            payment = unsettledBalance;
        } else {
           payment = this.balance;
        }
        matchedProvider.transfer(payment);
        // update the total amount billed and unsetted
        totalBilled += payment;
        unsettledBalance -= payment;

        // if unselted balance is > N hour value of contract let provider cancel it
        if (unsettledBalance > maxUnsettledBalance) {
            delinquent = true;
        }
    }

    // reset contract fields. Prevents payments. Sends contract balance to maintainer
    function cancel() public onlyMaintainer notCanceled notDelinquent {
        matched = false;
        canceled = true;
        maintainer.transfer(this.balance);
    }

    // lets the provider cancel the contract for deliquent payment
    // leave delinquent true so there is a perminent record of clients bad behavior
    function providerCancel() public isDelinquent {
        // only matchedProvider should be able to call this
        require(msg.sender == address(matchedProvider));
        matched = false;
        canceled = true;
        matchedProvider.transfer(this.balance);
    }

    // makes canceled false. contract can be rematched to a provider.
    function uncancel() public isCanceled notDelinquent{
        canceled = false;
    }
}

contract Master is Maintainable {

    function Master() public Maintainable(msg.sender) {}

    // call to put a bid for a service on the network
    function deployProvider(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee, string _networkAddress) public payable returns (Provider) {
        // provider must provide the cancel fee up front
        require(msg.value >= _cancelFee);
        Provider provider = new Provider(msg.sender, _ram, _cpu, _rate, _minimumBalanace, _cancelFee, _networkAddress);
        provider.transfer(msg.value);
        return provider;
    }

    // call to put an ask for a service on the network
    function deployClient(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee, uint _maxUnsettledBalance) public payable returns (Client) {
        // ensure client contract has minimum balance
        require(msg.value >= _minimumBalanace);
        Client client = new Client(msg.sender, _ram, _cpu, _rate, _minimumBalanace, _cancelFee, _maxUnsettledBalance);
        client.transfer(msg.value);
        return client;
    }

    // try to maych a client and a provider
    // this is onlyMaintainer because the matching must be done in a fair way
    // photon will do the matching based on provider and client order sequennces
    // the results of the matching are publicly auditable because eth transactions are public
    function matchContracts(address clientAddress, address providerAddress) onlyMaintainer public {
        Provider provider = Provider(providerAddress);
        Client client = Client(clientAddress);
        // check if provider can match with client and  match provider with client
        require(provider.matchClient(clientAddress));
        // match client with provider
        client.matchProvider(providerAddress);
    }
}
