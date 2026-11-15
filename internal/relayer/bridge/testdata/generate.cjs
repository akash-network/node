// Development oracle only. Run from repository root with the pinned node_modules path.
// AKT entries below encode the actual Rust declarations, not a generated IDL.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { createRequire } = require('node:module');
const req = createRequire(path.join(path.resolve(process.argv[2]), 'package.json'));
const anchor = req('@coral-xyz/anchor');
const { PublicKey, Ed25519Program, AddressLookupTableProgram, ComputeBudgetProgram, SystemProgram, SYSVAR_INSTRUCTIONS_PUBKEY } = req('@solana/web3.js');
const { TOKEN_PROGRAM_ID, ASSOCIATED_TOKEN_PROGRAM_ID, getAssociatedTokenAddressSync } = req('@solana/spl-token');
const { fromBech32 } = req('@cosmjs/encoding');
const u64 = n => { const b=Buffer.alloc(8); b.writeBigUInt64LE(BigInt(n)); return b; };
const u32 = n => { const b=Buffer.alloc(4); b.writeUInt32LE(n); return b; };
const vec = b => Buffer.concat([u32(b.length),Buffer.from(b)]);
const key = n => new PublicKey(Buffer.alloc(32,n));
const bn = n => new anchor.BN(String(n));
const hash = b => crypto.createHash('sha256').update(b).digest();
const disc = (kind,name) => hash(Buffer.from(kind+':'+name)).subarray(0,8);
const pda = (program,...seeds) => PublicKey.findProgramAddressSync(seeds.map(s=>typeof s==='string'?Buffer.from(s):s),program)[0];
const program=key(1), client=key(2), access=key(3), payer=key(4), recipient=key(5), session=Buffer.alloc(32,6);
const clientID = JSON.parse(fs.readFileSync('contracts/solana/idls/ics07_tendermint.json'));
const accessID = JSON.parse(fs.readFileSync('contracts/solana/idls/access_manager.json'));
const proof = JSON.parse(fs.readFileSync('contracts/solana/programs/akt/tests/fixtures/proof_vectors.json'))[0];
const owner=Buffer.from(proof.owner_hex,'hex'), h=BigInt(proof.height), seq=BigInt(proof.sequence), nativeKey=Buffer.from(proof.transfer_key_hex,'hex');
const id=Buffer.concat([vec(owner),u64(h),u64(seq)]);
const addr={config:pda(program,'config'),bridge:pda(program,'bridge'),mint:pda(program,'mint'),mint_authority:pda(program,'mint_authority'),status:pda(program,'transfer',hash(owner),u64(h),u64(seq)),proof_buffer:pda(program,'proof',payer.toBuffer(),hash(nativeKey)),client_state:pda(client,'client'),app_state:pda(client,'app_state'),access_manager:pda(access,'access_manager'),trusted_consensus_state:pda(client,'consensus_state',u64(19)),new_consensus_state_store:pda(client,'consensus_state',u64(20)),signature_batch:pda(client,'sig_batch',payer.toBuffer(),session),chunk:pda(client,'header_chunk',payer.toBuffer(),u64(20),Buffer.from([0]))};
const loader=new PublicKey('BPFLoaderUpgradeab1e11111111111111111111111');
const pd = prog=>pda(loader,prog.toBuffer());
const receipt={signature_hash:Array(32).fill(9),pubkey:Array(32).fill(10),msg:Buffer.from([11,12,13]),signature:Array(64).fill(14)};
const clientState={chain_id:'akash-local-1',trust_level_numerator:bn(1),trust_level_denominator:bn(3),trusting_period:bn('18446744073709551615'),unbonding_period:bn(1814400),max_clock_drift:bn(10),frozen_height:{revision_number:bn(0),revision_height:bn(0)},latest_height:{revision_number:bn(1),revision_height:bn(20)}};
const consensus={timestamp:bn('18446744073709551615'),root:Array(32).fill(7),next_validators_hash:Array(32).fill(8)};
const out={source:'Pinned Anchor/TS IDLs and web3.js. AKT vectors hand-encoded from pinned owning Rust declarations.',versions:{anchor:req('@coral-xyz/anchor/package.json').version,web3:req('@solana/web3.js/package.json').version},sourceHashes:{},id:proof.id,owner:proof.owner,owner_hex:proof.owner_hex,height:proof.height,sequence:proof.sequence,nativeKey:proof.transfer_key_hex,addresses:{},instructions:{},accounts:{}};
const curveBytes=[Buffer.alloc(32),Buffer.from([1,...Array(30).fill(0),128]),Buffer.from('edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f','hex'),Buffer.from('eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f','hex')];
for(let i=0;i<128;i++) curveBytes.push(hash(Buffer.from('bridge-curve-oracle-'+i)));
out.curves=curveBytes.map(b=>({hex:b.toString('hex'),onCurve:PublicKey.isOnCurve(b)}));
for (const [n,a] of Object.entries(addr)) out.addresses[n]=a.toBase58();
for (const source of ['contracts/solana/idls/ics07_tendermint.json','contracts/solana/idls/access_manager.json','contracts/solana/programs/akt/src/state.rs','contracts/solana/programs/akt/src/wire.rs','contracts/solana/programs/akt/src/lib.rs',...['mint_transfer','void_transfer','proof_buffer','genesis','bridge_admin','upgrade_authority'].map(name=>'contracts/solana/programs/akt/src/instructions/'+name+'.rs'),'contracts/solana/programs/akt/tests/fixtures/proof_vectors.json']) out.sourceHashes[source]=hash(fs.readFileSync(source)).toString('hex');
function save(name,program,data,keys){ out.instructions[name]={program:program.toBase58(),data:data.toString('hex'),keys:keys.map(k=>({key:k.pubkey.toBase58(),signer:k.isSigner,writable:k.isWritable}))}; }
const meta=(pubkey,writable=false,signer=false)=>({pubkey,isWritable:writable,isSigner:signer});
function idlInstruction(label,idl,prog,name,args,accounts,remaining=[]){
 const coder=new anchor.BorshInstructionCoder(idl), data=coder.encode(name,args), decoded=coder.decode(data);
 if (!decoded || !coder.encode(name,decoded.data).equals(data)) throw Error('IDL roundtrip '+name);
 const definition=idl.instructions.find(i=>i.name===name);
 save(label,prog,data,[...definition.accounts.map(a=>meta(a.address?new PublicKey(a.address):accounts[a.name],a.writable===true,a.signer===true)),...remaining]);
}
const common={...addr,submitter:payer,payer,authority:payer,program_data:pd(client),consensus_state_store:addr.new_consensus_state_store,client_state_account:addr.client_state};
idlInstruction('signature-create',clientID,client,'initialize_signature_batch',{session:[...session]},common);
idlInstruction('signature',clientID,client,'pre_verify_signature_batch',{signature:receipt},common);
idlInstruction('chunk',clientID,client,'upload_header_chunk',{params:{target_height:bn(20),chunk_index:0,chunk_data:Buffer.from([1,2,3])}},common);
idlInstruction('assembly',clientID,client,'assemble_and_update_client',{target_height:bn(20),chunk_count:1,trusted_height:bn(19)},common,[meta(addr.chunk,true),meta(addr.signature_batch,true)]);
idlInstruction('cleanup',clientID,client,'cleanup_incomplete_upload',{},common,[meta(addr.chunk,true),meta(addr.signature_batch,true)]);
idlInstruction('prune',clientID,client,'prune_consensus_state',{height:bn(20)},common);
idlInstruction('client-initialize',clientID,client,'initialize',{client_state:clientState,consensus_state:consensus,access_manager:access},common);
idlInstruction('access-initialize',accessID,access,'initialize',{admin:payer},{...common,program_data:pd(access)});
idlInstruction('grant-role',accessID,access,'grant_role',{role_id:bn(1),account:payer},{...common,admin:payer});
// Latest height 20 and proof height 19 keep the two consensus metas distinguishable.
const akt=(name,args,metas)=>save(name,program,Buffer.concat([disc('global',name),...args]),metas);
akt('create_proof_buffer',[hash(nativeKey),u32(1234)],[meta(payer,true,true),meta(addr.proof_buffer,true),meta(SystemProgram.programId)]);
akt('grow_proof_buffer',[],[meta(payer,true,true),meta(addr.proof_buffer,true),meta(SystemProgram.programId)]);
akt('append_proof',[u32(450),vec(Buffer.from([1,2,3]))],[meta(payer,false,true),meta(addr.proof_buffer,true)]);
akt('close_proof_buffer',[],[meta(payer,true,true),meta(addr.proof_buffer,true)]);
akt('mint_transfer',[id,recipient.toBuffer(),u64('18446744073709551615'),u64(19)],[meta(payer,true,true),meta(addr.config),meta(addr.bridge,true),meta(addr.proof_buffer),meta(addr.status,true),meta(client),meta(addr.client_state),meta(addr.new_consensus_state_store),meta(addr.trusted_consensus_state),meta(addr.mint,true),meta(addr.mint_authority),meta(recipient),meta(getAssociatedTokenAddressSync(addr.mint,recipient,true),true),meta(TOKEN_PROGRAM_ID),meta(ASSOCIATED_TOKEN_PROGRAM_ID),meta(SystemProgram.programId)]);
akt('void_transfer',[id,Buffer.from([5]),u64(19)],[meta(payer,true,true),meta(addr.bridge),meta(addr.proof_buffer),meta(addr.status,true),meta(client),meta(addr.client_state),meta(addr.new_consensus_state_store),meta(addr.trusted_consensus_state),meta(SystemProgram.programId)]);
akt('initialize',[],[meta(program),meta(pd(program)),meta(payer,true,true),meta(addr.config,true),meta(addr.mint,true),meta(addr.mint_authority),meta(TOKEN_PROGRAM_ID),meta(SystemProgram.programId)]);
akt('configure_bridge',[vec(Buffer.from('akash-local-1')),u64(1000),u64(10000),u32(1),recipient.toBuffer()],[meta(program),meta(pd(program)),meta(payer,true,true),meta(addr.config),meta(addr.bridge,true),meta(client),meta(addr.client_state),meta(addr.new_consensus_state_store),meta(SystemProgram.programId)]);
akt('set_bridge_enabled',[Buffer.from([1])],[meta(program),meta(pd(program)),meta(payer,true,true),meta(addr.config,true),meta(addr.bridge,true)]);
const systemIx=(label,ix)=>save(label,ix.programId,ix.data,ix.keys);
systemIx('ed25519',Ed25519Program.createInstructionWithPublicKey({publicKey:Uint8Array.from(receipt.pubkey),message:receipt.msg,signature:Uint8Array.from(receipt.signature)}));
systemIx('compute-limit',ComputeBudgetProgram.setComputeUnitLimit({units:1400000}));
systemIx('compute-heap',ComputeBudgetProgram.requestHeapFrame({bytes:262144}));
const [create,lookup]=AddressLookupTableProgram.createLookupTable({authority:payer,payer,recentSlot:123});out.addresses.lookup=lookup.toBase58();
systemIx('lookup-create',create);
systemIx('lookup-extend',AddressLookupTableProgram.extendLookupTable({lookupTable:lookup,authority:payer,payer,addresses:[addr.chunk,addr.signature_batch]}));
systemIx('lookup-deactivate',AddressLookupTableProgram.deactivateLookupTable({lookupTable:lookup,authority:payer}));
systemIx('lookup-close',AddressLookupTableProgram.closeLookupTable({lookupTable:lookup,authority:payer,recipient:payer}));
(async()=>{
 for(const [idl,states] of [[clientID,{ClientState:clientState,ConsensusStateStore:{height:bn(20),consensus_state:consensus},AppState:{am_state:{access_manager:access,pending_access_manager:recipient},_reserved:Array(256).fill(17)},HeaderChunk:{submitter:payer,chunk_data:Buffer.from([1,2,3])},SignatureBatch:{version:1,submitter:payer,session:[...session],bump:254,hashes:[Array(32).fill(9),Array(32).fill(10)]}}],[accessID,{AccessManager:{roles:[{role_id:bn(1),members:[client,payer]}],whitelisted_programs:[recipient],pending_authority_transfers:[{target_program:client,new_authority:recipient}]}}]]) {
  const coder=new anchor.BorshAccountsCoder(idl);
  for(const [name,value] of Object.entries(states)) {
   const bytes=await coder.encode(name,value); const decoded=coder.decode(name,bytes);
   if(!(await coder.encode(name,decoded)).equals(bytes)) throw Error('account roundtrip '+name);
   out.accounts[name]=bytes.toString('hex');
  }
 }
 const acc=(name,parts)=>out.accounts[name]=Buffer.concat([disc('account',name),...parts]).toString('hex');
 acc('LaunchConfig',[Buffer.from([2]),payer.toBuffer(),addr.mint.toBuffer(),Buffer.from([1]),u64(42),Buffer.from([253,252])]);
 acc('BridgeConfig',[client.toBuffer(),vec(Buffer.from('akash-local-1')),u64(1000),u64(10000),u32(1),recipient.toBuffer(),u64(7),u64(900)]);
 acc('ProofBuffer',[payer.toBuffer(),hash(nativeKey),u32(1234),vec(Buffer.from([1,2,3]))]);
 acc('TransferStatus',[id,u64(20),u64(42),Buffer.from([0]),u64('18446744073709551615'),recipient.toBuffer()]);
 out.accounts.TransferStatusVoided=Buffer.concat([disc('account','TransferStatus'),id,u64(20),u64(42),Buffer.from([1,1])]).toString('hex');
 fs.writeFileSync(process.argv[3] ?? 'internal/relayer/bridge/testdata/wire.json',JSON.stringify(out,null,2)+'\n');
})().catch(e=>{console.error(e);process.exitCode=1});
