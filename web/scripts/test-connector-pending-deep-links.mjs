import assert from 'node:assert/strict';
import { load, jsx, ui, harness, nodes, flush } from './connector-fixture.mjs';

const connector={id:'feishu',name:'Feishu'};
const id='11111111-1111-4111-8111-111111111111';
let owner='owner',org='org',reads=0,pending,reply={data:{id,connector_id:'feishu'}},redirects=[];
const router={replace:path=>redirects.push(path)};
const mount=()=>harness(react=>{
  const {ConnectorDeepLink}=load('src/components/connectors/detail/connector-deep-link.tsx',{
    react,'react/jsx-runtime':jsx,'next/navigation':{useRouter:()=>router},
    '@/store/auth-store':{useAuthStore:{use:{user:()=>({id:owner})},getState:()=>({user:{id:owner}})}},
    '@/store/organization-store':{useOrganizationStore:{use:{currentOrganization:()=>({id:org})},getState:()=>({currentOrganization:{id:org}})}},
    '@/i18n':{useT:()=>key=>key},
    '@/hooks/connectors/use-connectors':{useConnector:()=>({data:{data:connector}})},
    '@/hooks/connectors/use-pending-setups':{usePendingSetup:()=>({data:pending?{data:pending}:undefined})},
    '@/services/connector.service':{connectorService:{getConnection:async()=>{reads++;return reply;}}},
    '@/components/connectors/setup/connector-create-dialog':ui,
    '@/components/connectors/connector-page-state':ui,
  });
  return props=>ConnectorDeepLink(props);
});
let h=mount();h.render({connectorId:'feishu',connectionID:id});await flush();
assert.deepEqual(redirects,[`/console/connectors/connections/${id}?action=credentials`]);
assert.equal(reads,1);h.unmount();redirects=[];
h=mount();h.render({connectorId:'feishu',connectionID:id,pendingID:id});await flush();
assert.equal(reads,1,'ambiguous link does not look up or mutate anything');assert.equal(nodes(h.value)[0].type,'ConnectorPageMessage');h.unmount();
reply={data:{id,connector_id:'github'}};
h=mount();h.render({connectorId:'feishu',connectionID:id});await flush();h.render({connectorId:'feishu',connectionID:id});
assert.equal(redirects.length,0);assert.equal(nodes(h.value)[0].type,'ConnectorPageMessage');h.unmount();
let resolve;reply=new Promise(done=>{resolve=done;});
h=mount();h.render({connectorId:'feishu',connectionID:id});owner='other';resolve({data:{id,connector_id:'feishu'}});await flush();
assert.equal(redirects.length,0,'late owner response cannot navigate to credentials');h.unmount();owner='owner';
pending={kind:'pending_setup',id,connector_id:'feishu',app_id:'cli_original'};
h=mount();h.render({connectorId:'feishu',pendingID:id});
const dialog=nodes(h.value).find(node=>node.type==='ConnectorCreateDialog');assert.equal(dialog.props.pendingSetupID,id);
const before=reads;dialog.props.onOpenChange(false);assert.equal(reads,before);assert.deepEqual(redirects,['/console/connectors/feishu']);h.unmount();

const route=load('src/app/console/connectors/connections/[connectionId]/page.tsx',{
  react:{use:value=>value},'react/jsx-runtime':jsx,
  '@/components/connectors/connections/connection-management-page':ui,
}).default;
const both=route({params:{connectionId:id},searchParams:{rename:'1',action:'credentials'}});
assert.equal(both.props.credentialsInitiallyOpen,true);assert.equal(both.props.renameInitiallyOpen,false);
const rename=route({params:{connectionId:id},searchParams:{rename:'1'}});assert.equal(rename.props.renameInitiallyOpen,true);
console.log('Connector deep links: original credential dialog, identity/owner checks, GET-only pending, and action precedence passed.');
