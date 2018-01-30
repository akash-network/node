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

 contract Client is Ownable, Parameterized, Matchable, Cancelable, Payable, Delinquent {

    Provider public matchedProvider;
    uint public matchStartTime;
    uint public totalBilled;
    uint public unsettledBalance;
    uint public maxUnsettledBalance;
    string public manifest;

    // Constructor
    function Client(address maintainer, uint ram, uint cpu, uint rate, uint _minimumBalanace, uint cancelFee, uint _maxUnsettledBalance, string _manifest) public payable Parameterized(ram, cpu, rate, _minimumBalanace, cancelFee) Maintainable(maintainer) {
        maxUnsettledBalance = _maxUnsettledBalance;
        manifest = _manifest;
    }

    // match with provider
    function matchProvider(address providerAddress) external onlyOwner notCanceled notMatched returns (bool) {
        matchedProvider = Provider(providerAddress);
        matchStartTime = now;
        matched = true;
        return matched;
    }

    // send the value of the outstanding bill to the provider
    function bill() public notCanceled {
         uint payment;
        // pay required amount since last payment time
        // get the required payment by total payment - expected payment
        unsettledBalance = (now - matchStartTime) * rate - totalBilled;
        // pay the provider
        if (this.balance > unsettledBalance) {
            payment = unsettledBalance;
        } else {
            payment = this.balance;
        }
        // update the total amount billed and unsetted
        totalBilled += payment;
        unsettledBalance -= payment;
        // send payment
        matchedProvider.transfer(payment);

        // if unselted balance is greater than maxUnsettledBalance
        // let provider cancel it by setting delinquent to true
        if (unsettledBalance > maxUnsettledBalance) {
            delinquent = true;
        }
    }

    // allow maintainer to withdrawal ETH
    function withdrawal(uint amount) public onlyMaintainer notDelinquent {
        maintainer.transfer(amount);
    }

    // reset contract fields
    function reset() private {
        matched = false;
        canceled = true;
        matchedProvider = Provider(0x0);
    }

    // reset contract fields. Prevents payments.
    function cancel() public onlyMaintainer notCanceled {
        matchedProvider.clientCancel();
        reset();
    }

    // lets the provider cancel the contract for deliquent payment
    // leave delinquent true so there is a perminent record of clients bad behavior
    function providerCancel() public isDelinquent {
        // only matchedProvider should be able to call this
        require(msg.sender == address(matchedProvider));
        reset();
        matchedProvider.transfer(this.balance);
    }

    // makes canceled false. contract can be rematched to a provider.
    function uncancel() public onlyMaintainer isCanceled notDelinquent {
        canceled = false;
    }

    function () public payable {}
}

contract Provider is Ownable, Parameterized, Matchable, Cancelable, Payable, BadActor {

    Client public matchedClient;
    string public networkAddress;

    // Constructor
    function Provider(address maintainer, uint ram, uint cpu, uint rate, uint minimumBalanace, uint cancelFee, string _networkAddress) public Parameterized(ram, cpu, rate, minimumBalanace, cancelFee) Maintainable(maintainer) {
        networkAddress = _networkAddress;
    }

    // confirm and declare that a valid matching contract is at an address
    function matchClient(address clientAddress) external onlyOwner notCanceled notMatched returns (bool) {
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
        canceled = true;
        matchedClient = Client(0x0);
    }

    // maintainer cancels contract and incurrs a fee
    function earlyCancel() public onlyMaintainer notCanceled  {
        reset();
        // one last bill to the client
        matchedClient.bill();
        // if the matchedClient is not delinquent charge an early cancellation fee
        if (!matchedClient.delinquent()) {
            matchedClient.transfer(cancelFee);
        }
    }

    // client cancels contract
    function clientCancel() external notCanceled {
        // only matchedClient should be able to call this
        require(msg.sender == address(matchedClient));
        // one last bill to the client
        matchedClient.bill();
        reset();
    }

    // makes canceled false. contract can be rematched to a provider.
    function uncancel() public isCanceled {
        canceled = false;
    }

    // send maximum allowable funds to contract maintainer
    function withdrawal() public {
         uint amount = this.balance;
        if (!canceled) {
            // ensure contract always can pay the early cancel fee when not canceled
            amount = amount - cancelFee;
        }
        require(amount > 0);
        maintainer.transfer(amount);
    }
}

contract Master is Maintainable {

    function Master() public Maintainable(msg.sender) {}

     // call to put an ask for a service on the network
    function deployClient(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee, uint _maxUnsettledBalance, string _manifest) public payable returns (Client) {
        // ensure client contract has minimum balance
        require(msg.value >= _minimumBalanace);
        Client client = new Client(msg.sender, _ram, _cpu, _rate, _minimumBalanace, _cancelFee, _maxUnsettledBalance, _manifest);
        client.transfer(msg.value);
        return client;
    }

    // call to put a bid for a service on the network
    function deployProvider(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee, string _networkAddress) public payable returns (Provider) {
        // provider must provide the cancel fee up front
        require(msg.value >= _cancelFee);
        Provider provider = new Provider(msg.sender, _ram, _cpu, _rate, _minimumBalanace, _cancelFee, _networkAddress);
        provider.transfer(msg.value);
        return provider;
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
