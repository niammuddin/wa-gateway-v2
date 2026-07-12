function app(){return{
 token:'',refreshPromise:null,bootstrapPromise:null,refreshTimer:null,pollTimer:null,sessionPollTimer:null,qrTimer:null,qrTick:0,username:'',password:'',error:'',
 page:'dashboard',sidebarOpen:false,
 sessions:[],messages:[],apiKeys:[],templates:[],webhooks:[],webhookDeliveries:[],webhookDeliveryStatus:'',webhookDeliveryWebhook:'',webhookDeliverySearch:'',webhookDeliveryPage:1,webhookDeliveryLimit:20,webhookDeliveryTotal:0,webhookDeliveryStats:{total:0,delivered:0,failed:0,retrying:0,queued:0},
 dashboard:{activeSessions:0,messagesToday:0,failedMessages:0,queueSize:0,apiUsage:0},
  mon:{status:'',services:{}},queue:{counts:{waiting:0,active:0,completed:0,failed:0,delayed:0},jobs:[],page:1,limit:10,total:0},apiKeyReveal:'',
 stats:{totalMessages:0,successCount:0,failedCount:0,successRate:0,todayMessages:0,activeSessions:0,dailyBreakdown:[],bySession:[]},overviewSessionId:'',queueFilters:{sessionId:'',status:'',search:''},msgFilters:{search:'',status:'',sessionId:'',page:1,limit:20,total:0},activeMsg:null,drawer:false,activeWebhookDelivery:null,webhookDrawer:false,activeQueueJob:null,queueDrawer:false,
 dashboardStats: [
  {k:'activeSessions',l:'Sessions',c:'text-green-600',b:'bg-green-50',icon:'<path d="M12 2C6.477 2 2 6.477 2 12s4.477 10 10 10 10-4.477 10-10S17.523 2 12 2zm-1.5 13.5L7 12l1.414-1.414L10.5 12.67l4.586-4.586L16.5 9.5l-6 6z" fill="currentColor"/>'},
  {k:'messagesToday',l:'Today',c:'text-blue-600',b:'bg-blue-50',icon:'<path d="M20 2H4c-1.1 0-1.99.9-1.99 2L2 22l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zM6 9h12v2H6V9zm8 5H6v-2h8v2zm4-6H6V6h12v2z" fill="currentColor"/>'},
  {k:'failedMessages',l:'Failed',c:'text-red-500',b:'bg-red-50',icon:'<path d="M12 2C6.47 2 2 6.47 2 12s4.47 10 10 10 10-4.47 10-10S17.53 2 12 2zm5 13.59L15.59 17 12 13.41 8.41 17 7 15.59 10.59 12 7 8.41 8.41 7 12 10.59 15.59 7 17 8.41 13.41 12 17 15.59z" fill="currentColor"/>'},
  {k:'queueSize',l:'Queue',c:'text-yellow-600',b:'bg-yellow-50',icon:'<path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12z" fill="currentColor"/>'},
  {k:'apiUsage',l:'API Calls',c:'text-brand-600',b:'bg-brand-50',icon:'<path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-4 6h-4v2h4v2h-4v2h4v2H9V7h6v2z" fill="currentColor"/>'}
 ],
 notif:{text:'',type:'success',show:false,icon:'',progress:100},
 modal:{show:false,title:'',form:''},
 dialog:{show:false,title:'',msg:'',variant:'danger',confirmText:'Delete',cb:null},
 form:{sessionId:'',method:'qr',phoneNumber:'',name:'',rateLimit:null,allowedIps:'',body:'',url:'',events:'',selectedEvents:[],secret:'',sessionIds:'',apiKeyId:'',minIntervalMs:3000,jitterMs:2000,maxMessagesPerMinute:20,failureThreshold:5,pauseDurationMs:300000,isActive:true,editId:null},
 eventOptions:[
  {name:'message.sent',description:'Pesan berhasil dikirim ke WhatsApp.'},
  {name:'message.delivered',description:'Pesan sudah diterima perangkat tujuan.'},
  {name:'message.read',description:'Pesan sudah dibaca oleh penerima.'},
  {name:'message.failed',description:'Pesan gagal setelah seluruh retry selesai.'},
  {name:'session.connected',description:'Session WhatsApp berhasil terhubung.'},
  {name:'session.disconnected',description:'Session terputus atau logout.'},
 ],
 navItems:[
  {page:'dashboard',label:'Dashboard',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>'},
  {page:'sessions',label:'Sessions',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><rect x="4" y="2" width="16" height="20" rx="2"/><line x1="8" y1="6" x2="16" y2="6"/><line x1="8" y1="10" x2="16" y2="10"/><line x1="8" y1="14" x2="12" y2="14"/></svg>'},
  {page:'messages',label:'Messages',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z"/></svg>'},
  {page:'apikeys',label:'API Keys',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 11-7.778 7.778 5.5 5.5 0 017.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>'},
  {page:'templates',label:'Templates',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>'},
  {page:'webhooks',label:'Webhooks',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M18 8A6 6 0 006 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 01-3.46 0"/></svg>'},
  {page:'queue',label:'Queue',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>'},
  {page:'stats',label:'Statistics',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>'},
  {page:'monitoring',label:'Monitoring',icon:'<svg class="sidebar-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>'},
 ],
 h(isJson){const h={'Authorization':'Bearer '+this.token};if(isJson)h['Content-Type']='application/json';return h},
 clearTimers(){
   if (this.refreshTimer) clearTimeout(this.refreshTimer);
   if (this.pollTimer) clearInterval(this.pollTimer);
   if (this.sessionPollTimer) clearTimeout(this.sessionPollTimer);
   if (this.qrTimer) clearInterval(this.qrTimer);
   this.refreshTimer = null;
   this.pollTimer = null;
   this.sessionPollTimer = null;
   this.qrTimer = null;
 },
 scheduleRefresh(expiresIn){
   if (this.refreshTimer) clearTimeout(this.refreshTimer);
   const seconds = Math.max(Number(expiresIn) || 900, 60);
   const delay = Math.max((seconds - 30) * 1000, 1000);
   this.refreshTimer = setTimeout(async () => {
     const refreshed = await this.refreshAuth();
     if (!refreshed) this.logout();
   }, delay);
 },
 async persistTokens(tokens){
   this.token=tokens.accessToken||'';
   this.scheduleRefresh(tokens.expiresIn);
 },
 async bootstrapAuth(){
   if (!this.bootstrapPromise) {
     this.bootstrapPromise = (async () => {
       try {
         const res = await fetch('/api/v1/auth/session', {
           method: 'GET',
           credentials: 'include',
         });
         if (res.status === 204) return false;
         if (!res.ok) return false;
         const data = await res.json();
         await this.persistTokens(data);
         return true;
       } catch {
         return false;
       }
     })().finally(() => {
       this.bootstrapPromise = null;
     });
   }
   return this.bootstrapPromise;
 },
 async refreshAuth(){
   if (!this.refreshPromise) {
     this.refreshPromise = (async () => {
       try {
         const res = await fetch('/api/v1/auth/refresh', {
           method: 'POST',
           credentials: 'include'
         });
         if (!res.ok) return false;
         const data = await res.json();
         await this.persistTokens(data);
         return true;
       } catch {
         return false;
       }
     })().finally(() => {
       this.refreshPromise = null;
     });
   }
   return this.refreshPromise;
 },
 async api(url,opts={},retry=true){if(!this.token&&!url.startsWith('/auth')){const bootstrapped=await this.bootstrapAuth();if(!bootstrapped)return null}
  const bh=url.startsWith('/auth')?{'Content-Type':'application/json'}:this.h(!!opts.body);
  try{const res=await fetch('/api/v1'+url,{...opts,headers:{...bh,...(opts.headers||{})}});
  if(res.status===401&&!url.startsWith('/auth')&&retry){const refreshed=await this.refreshAuth();if(refreshed)return this.api(url,opts,false);this.msg('Sesi habis','error');this.logout();return null}
  if(res.status===401){this.msg('Sesi habis','error');this.logout();return null}
  const t=await res.text();let d;try{d=JSON.parse(t)}catch{d=null}
  if(!res.ok)throw new Error((d&&d.message)||'HTTP '+res.status);return d}catch(err){this.msg(err.message||'Gagal','error');throw err}},
 msg(text, type='success') {
      const icons = {
        success: `<div class="w-6 h-6 rounded-full bg-green-100 flex items-center justify-center text-green-500 shrink-0">
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
        </div>`,
        error: `<div class="w-6 h-6 rounded-full bg-red-100 flex items-center justify-center text-red-500 shrink-0">
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </div>`,
        warning: `<div class="w-6 h-6 rounded-full bg-yellow-100 flex items-center justify-center text-yellow-500 shrink-0">
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
        </div>`,
        info: `<div class="w-6 h-6 rounded-full bg-blue-100 flex items-center justify-center text-blue-500 shrink-0">
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
        </div>`
      };

      if (window._notifInterval) clearInterval(window._notifInterval);
      if (window._notifTimeout) clearTimeout(window._notifTimeout);

      this.notif = { text, type, show: true, icon: icons[type] || icons.success, progress: 100 };
      
      const duration = 3000;
      const start = Date.now();
      
      window._notifInterval = setInterval(() => {
        const elapsed = Date.now() - start;
        this.notif.progress = Math.max(0, 100 - (elapsed / duration) * 100);
      }, 30);

      window._notifTimeout = setTimeout(() => {
        this.notif.show = false;
        clearInterval(window._notifInterval);
      }, duration);
    },
 conf(title,msg,variant,confirmText,cb){this.dialog={show:true,title,msg,variant:variant||'danger',confirmText,cb}},

      async sendTestMessage() {
        const { form } = this;
        if (!form.sessionId || !form.testTo.trim() || (!form.testMsg.trim() && form.testType === 'text')) {
          this.msg('Lengkapi form kirim pesan', 'error');
          return;
        }
        try {
          const body = {
            sessionId: form.sessionId,
            to: form.testTo,
            type: form.testType,
            message: form.testMsg || undefined,
            url: form.testUrl || undefined,
            filename: form.testFilename || undefined
          };
          const res = await this.api('/messages/send', { method: 'POST', body: JSON.stringify(body) });
          if (res) {
            this.msg('Pesan berhasil masuk antrean queue');
            this.modal.show = false;
            await this.loadAll();
          }
        } catch (err) { /* error shown by api() */ }
      },
      
      openSendTestModal() {
        this.openModal('sendtest');
        const connected = this.sessions.filter(x => x.status === 'connected');
        if (connected.length > 0) this.form.sessionId = connected[0].session_id;
      },
      async resendMessage(id) {
        try {
          const res = await this.api('/messages/' + id + '/resend', { method: 'POST' });
          if (res) {
            this.showMessage('Pesan masuk antrean resend');
            this.modal.show = false;
            await this.loadAll();
          }
        } catch (err) {}
      },
      openMsgDetail(m) {
        this.activeMsg = m;
        this.drawer = true;
      },
      async deleteMessage(id) {
        const res = await this.api('/messages/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!res) return;
        this.messages = this.messages.filter(message => message.id !== id);
        this.msgFilters.total = Math.max((this.msgFilters.total || 0) - 1, 0);
        this.closeDrawer();
        this.msg('Message deleted');
      },
      closeDrawer() { this.drawer = false; },
 openModal(form,data){
   this.form={sessionId:'',method:'qr',phoneNumber:'',name:'',rateLimit:null,allowedIps:'',body:'',url:'',events:'',selectedEvents:[],secret:'',sessionIds:'',apiKeyId:'',minIntervalMs:3000,jitterMs:2000,maxMessagesPerMinute:20,failureThreshold:5,pauseDurationMs:300000,isActive:true,editId:null,testTo:'',testMsg:'',testType:'text',testUrl:'',testFilename:''};
   if(data){
     this.form.editId=data.id||data.session_id;
     this.form.sessionId=data.session_id||'';
     this.form.name=data.name||'';this.form.rateLimit=data.rate_limit||null;
     this.form.allowedIps=(data.allowed_ips||[]).join(', ');
     this.form.body=data.body||'';this.form.url=data.url||'';
     this.form.events=(data.events||[]).join(', ');
     this.form.selectedEvents=[...(data.events||[])];
     this.form.sessionIds=(data.session_ids||[]).join(', ');
     this.form.apiKeyId=data.api_key_id||'';
     this.form.minIntervalMs=data.min_interval_ms||3000;
     this.form.jitterMs=data.jitter_ms||2000;
     this.form.maxMessagesPerMinute=data.max_messages_per_minute||20;
     this.form.failureThreshold=data.failure_threshold||5;
     this.form.pauseDurationMs=data.pause_duration_ms||300000;
     this.form.isActive=data.is_active !== false;
   }
   const titles={session:'New Session','session-settings':'Pengaturan Session',apikey:data?'Edit API Key':'New API Key',template:data?'Edit Template':'New Template',webhook:data?'Edit Webhook':'New Webhook',sendtest:'Send Message'};
   this.modal={show:true,title:titles[form]||'',form};
 },

 async createSession(){
   const{form}=this;if(!form.sessionId.trim()){this.msg('ID wajib diisi','error');return}
   if(form.method==='pairing'&&!form.phoneNumber.trim()){this.msg('Phone wajib','error');return}
   const res=await this.api('/sessions',{method:'POST',body:JSON.stringify({sessionId:form.sessionId,method:form.method,phoneNumber:form.phoneNumber||undefined})});
   if(res){this.sessions.unshift(res);this.msg('Dibuat');this.watchSession(form.sessionId)}
 },
 async saveSessionThrottle(){
   const{form}=this;
   const res=await this.api('/sessions/'+encodeURIComponent(form.sessionId)+'/throttle',{method:'PUT',body:JSON.stringify({minIntervalMs:Number(form.minIntervalMs),jitterMs:Number(form.jitterMs),maxMessagesPerMinute:Number(form.maxMessagesPerMinute),failureThreshold:Number(form.failureThreshold),pauseDurationMs:Number(form.pauseDurationMs)})});
   if(res){await this.loadAll();this.msg('Pengaturan pengiriman disimpan')}
 },
 async deleteSession(sid){const session=this.sessions.find(s=>s.session_id===sid);if(Number(session?.message_count||0)>0){this.msg('Session memiliki riwayat message dan tidak dapat dihapus','error');return}const r=await this.api('/sessions/'+encodeURIComponent(sid),{method:'DELETE'});if(r){this.sessions=this.sessions.filter(s=>s.session_id!==sid);this.msg('Dihapus')}},
 async disconnectSession(sid){const r=await this.api('/sessions/'+encodeURIComponent(sid)+'/disconnect',{method:'POST'});if(r){this.msg('Disconnected');await this.loadAll()}},
 async logoutSession(sid){const r=await this.api('/sessions/'+encodeURIComponent(sid)+'/logout',{method:'POST'});if(r){this.msg('Device logged out');await this.loadAll()}},
 watchSession(sid){
   if(this.sessionPollTimer)clearTimeout(this.sessionPollTimer);
   let attempts=0;
   const poll=async()=>{
     await this.loadPageData('sessions',true);
     const session=this.sessions.find(item=>item.session_id===sid);
     if(!session||session.status==='connected'||session.status==='disconnected'||session.status==='failed'){this.sessionPollTimer=null;return}
     if(++attempts>=180){this.sessionPollTimer=null;return}
     this.sessionPollTimer=setTimeout(poll,500);
   };
   this.sessionPollTimer=setTimeout(poll,250);
 },
 async reconnectSession(sid){const r=await this.api('/sessions/'+encodeURIComponent(sid)+'/reconnect',{method:'POST'});if(!r)return;this.msg('Reconnecting...');this.watchSession(sid)},

 async saveApiKey(){
   const{form}=this;if(!form.name.trim()){this.msg('Nama wajib','error');return}
   const ips=form.allowedIps?form.allowedIps.split(',').map(i=>i.trim()).filter(i=>i):null;
   const body={name:form.name,rateLimit:form.rateLimit||null,allowedIps:ips,sessionId:form.sessionId||undefined,isActive:form.isActive};
   const res=form.editId?await this.api('/api-keys/'+form.editId,{method:'PUT',body:JSON.stringify(body)}):await this.api('/api-keys',{method:'POST',body:JSON.stringify(body)});
   if(res){if(!form.editId)this.apiKeyReveal=res.key||'';await this.loadAll();this.msg(form.editId?'Updated':'Created')}
 },
 async deleteApiKey(id){const r=await this.api('/api-keys/'+id,{method:'DELETE'});if(r){this.apiKeys=this.apiKeys.filter(k=>k.id!==id);this.msg('Deleted')}},

 async saveTemplate(){
   const{form}=this;if(!form.name.trim()||!form.body.trim()){this.msg('Nama & body wajib','error');return}
   const res=form.editId?await this.api('/templates/'+form.editId,{method:'PUT',body:JSON.stringify({name:form.name,body:form.body})}):await this.api('/templates',{method:'POST',body:JSON.stringify({name:form.name,body:form.body})});
   if(res){await this.loadAll();this.msg(form.editId?'Updated':'Created')}
 },
 async deleteTemplate(id){const r=await this.api('/templates/'+id,{method:'DELETE'});if(r){this.templates=this.templates.filter(t=>t.id!==id);this.msg('Deleted')}},

 async saveWebhook(){
   const{form}=this;if(!form.url.trim()||!(form.selectedEvents||[]).length){this.msg('URL & minimal satu event wajib','error');return}
   const events=[...(form.selectedEvents||[])];
   const sessionIds=form.sessionIds?form.sessionIds.split(',').map(i=>i.trim()).filter(i=>i):[];
   const payload={url:form.url,events,sessionIds,apiKeyId:form.apiKeyId||null,isActive:form.isActive};
   if(form.secret&&form.secret.trim()) payload.secret=form.secret.trim();
   const res=form.editId?await this.api('/webhooks/'+form.editId,{method:'PUT',body:JSON.stringify(payload)}):await this.api('/webhooks',{method:'POST',body:JSON.stringify(payload)});
   if(res){await this.loadAll();this.msg(form.editId?'Updated':'Created')}
 },
 async deleteWebhook(id){const r=await this.api('/webhooks/'+id,{method:'DELETE'});if(r){this.webhooks=this.webhooks.filter(w=>w.id!==id);this.msg('Deleted')}},
 async testWebhook(id){const r=await this.api('/webhooks/'+id+'/test',{method:'POST'});if(r){this.msg('Test webhook masuk antrean');await this.loadWebhookDeliveries()}},
 async retryWebhookDelivery(id){const r=await this.api('/webhooks/deliveries/'+id+'/retry',{method:'POST'});if(r){this.msg('Delivery dijadwalkan ulang');await this.loadWebhookDeliveries()}},
 openWebhookDetail(delivery){this.activeWebhookDelivery=delivery;this.webhookDrawer=true},
 async loadWebhookDeliveries(){
   const params=new URLSearchParams({page:String(this.webhookDeliveryPage||1),limit:String(this.webhookDeliveryLimit||20)});
   if(this.webhookDeliveryStatus)params.set('status',this.webhookDeliveryStatus);
   if(this.webhookDeliveryWebhook)params.set('webhookId',this.webhookDeliveryWebhook);
   if(this.webhookDeliverySearch.trim())params.set('search',this.webhookDeliverySearch.trim());
   const query='?'+params.toString();
   const res=await this.api('/webhooks/deliveries'+query);
   if(res){this.webhookDeliveries=res.data||[];this.webhookDeliveryTotal=res.total||0;}
   const stats=await this.api('/webhooks/stats');if(stats)this.webhookDeliveryStats={total:stats.total||0,...(stats.byStatus||{})};
 },
 webhookPageItems(){
   const totalPages=Math.max(Math.ceil((this.webhookDeliveryTotal||0)/(this.webhookDeliveryLimit||20)),1);
   const current=this.webhookDeliveryPage||1;
   const items=[];
   const pushPage=value=>items.push({key:'webhook-page-'+value,type:'page',value,label:value});
   const pushEllipsis=side=>items.push({key:'webhook-ellipsis-'+side+'-'+current+'-'+totalPages,type:'ellipsis'});
   if(totalPages<=7){for(let pageNumber=1;pageNumber<=totalPages;pageNumber++)pushPage(pageNumber);return items;}
   pushPage(1);
   const left=Math.max(current-1,2),right=Math.min(current+1,totalPages-1);
   if(left>2)pushEllipsis('left');
   for(let pageNumber=left;pageNumber<=right;pageNumber++)pushPage(pageNumber);
   if(right<totalPages-1)pushEllipsis('right');
   pushPage(totalPages);
   return items;
 },
 async goToWebhookPage(pageNumber){
   if(pageNumber==='...')return;
   const totalPages=Math.max(Math.ceil((this.webhookDeliveryTotal||0)/(this.webhookDeliveryLimit||20)),1);
   const nextPage=Math.min(Math.max(Number(pageNumber)||1,1),totalPages);
   if(nextPage===this.webhookDeliveryPage)return;
   this.webhookDeliveryPage=nextPage;
   await this.loadWebhookDeliveries();
 },
 async changeWebhookPage(delta){
   const nextPage=Math.max((this.webhookDeliveryPage||1)+delta,1);
   const maxPage=Math.max(Math.ceil((this.webhookDeliveryTotal||0)/(this.webhookDeliveryLimit||20)),1);
   if(nextPage===this.webhookDeliveryPage||nextPage>maxPage)return;
   this.webhookDeliveryPage=nextPage;
   await this.loadWebhookDeliveries();
 },
 deliveryStatusClass(status){
   return {delivered:'bg-green-50 text-green-700 ring-green-600/20',failed:'bg-red-50 text-red-700 ring-red-600/20',retrying:'bg-amber-50 text-amber-700 ring-amber-600/20',processing:'bg-blue-50 text-blue-700 ring-blue-600/20',queued:'bg-gray-50 text-gray-600 ring-gray-500/10',paused:'bg-gray-50 text-gray-500 ring-gray-500/10'}[status]||'bg-gray-50 text-gray-600 ring-gray-500/10';
 },
 formatPairingCode(code){return code?String(code).replace(/\s|-/g,'').replace(/^(.{4})(.*)$/, '$1-$2'):'-';},
 formatDateTime(value){return value?new Date(value).toLocaleString():'-';},

 async init(){
   localStorage.removeItem('token');
   localStorage.removeItem('refreshToken');
   this.clearTimers();
   if(!this.token){const bootstrapped=await this.bootstrapAuth();if(!bootstrapped)return}
   const savedPage=window.location.hash.replace(/^#/, '');
   const validPages=this.navItems.map(item=>item.page);
   if(validPages.includes(savedPage)) this.page=savedPage;
   await this.loadAll();
   this.connectEvents();
   this.qrTimer=setInterval(()=>{this.qrTick++},1000)
 },
 connectEvents(){
   if(this.events) this.events.close();
   this.events=new EventSource('/api/v1/events?token='+encodeURIComponent(this.token));
   this.events.onmessage=()=>this.loadAll();
   ['session.connected','session.disconnected','message.sent','message.delivered','message.read','message.failed'].forEach(name=>this.events.addEventListener(name,()=>this.loadAll()));
 },
 async setPage(pageName){
   this.page=pageName;
   window.history.replaceState(null,'','#'+pageName);
   await this.loadPageData(pageName,true);
 },
 async loadMessages(){
   const f=this.msgFilters;
   const query='/messages?page='+f.page+'&limit='+f.limit
     +(f.search?'&search='+encodeURIComponent(f.search):'')
     +(f.status?'&status='+encodeURIComponent(f.status):'')
     +(f.sessionId?'&sessionId='+encodeURIComponent(f.sessionId):'');
   const data=await this.api(query);if(data){this.messages=data.data||[];this.msgFilters.total=data.total||0;}
 },
 async loadDashboardMessages(){
   const sessionQuery=this.overviewSessionId?'&sessionId='+encodeURIComponent(this.overviewSessionId):'';
   const data=await this.api('/messages?page=1&limit=6'+sessionQuery);
   if(data)this.messages=data.data||[];
 },
 messagePageItems(){
   const totalPages=Math.max(Math.ceil((this.msgFilters.total||0)/(this.msgFilters.limit||20)),1);
   const current=this.msgFilters.page||1;
   const items=[];
   const pushPage=value=>items.push({key:'message-page-'+value,type:'page',value,label:value});
   const pushEllipsis=side=>items.push({key:'message-ellipsis-'+side+'-'+current+'-'+totalPages,type:'ellipsis'});
   if(totalPages<=7){for(let pageNumber=1;pageNumber<=totalPages;pageNumber++)pushPage(pageNumber);return items;}
   pushPage(1);
   const left=Math.max(current-1,2),right=Math.min(current+1,totalPages-1);
   if(left>2)pushEllipsis('left');
   for(let pageNumber=left;pageNumber<=right;pageNumber++)pushPage(pageNumber);
   if(right<totalPages-1)pushEllipsis('right');
   pushPage(totalPages);
   return items;
 },
 async goToMessagePage(pageNumber){
   const totalPages=Math.max(Math.ceil((this.msgFilters.total||0)/(this.msgFilters.limit||20)),1);
   const nextPage=Math.min(Math.max(Number(pageNumber)||1,1),totalPages);
   if(nextPage===this.msgFilters.page)return;
   this.msgFilters.page=nextPage;
   await this.loadMessages();
 },
 async changeMessagePage(delta){
   const nextPage=Math.max((this.msgFilters.page||1)+delta,1);
   const maxPage=Math.max(Math.ceil((this.msgFilters.total||0)/(this.msgFilters.limit||20)),1);
   if(nextPage===this.msgFilters.page||nextPage>maxPage)return;
   this.msgFilters.page=nextPage;
   await this.loadMessages();
 },
 async loadQueue(){
   const query='?page='+(this.queue.page||1)+'&limit='+(this.queue.limit||20)
     +(this.queueFilters.sessionId?'&sessionId='+encodeURIComponent(this.queueFilters.sessionId):'')
     +(this.queueFilters.status?'&status='+encodeURIComponent(this.queueFilters.status):'')
     +(this.queueFilters.search?'&search='+encodeURIComponent(this.queueFilters.search):'');
   const data=await this.api('/queue'+query);if(data){this.queue.counts=data.counts||this.queue.counts;this.queue.jobs=data.jobs||[];this.queue.page=data.page||1;this.queue.limit=data.limit||20;this.queue.total=data.total||0;}
 },
 async loadWebhookData(){
   const [webhooks,deliveries,stats]=await Promise.all([
     this.api('/webhooks'),
     this.api('/webhooks/deliveries?page='+(this.webhookDeliveryPage||1)+'&limit='+(this.webhookDeliveryLimit||20)),
     this.api('/webhooks/stats')
   ]);
   if(webhooks)this.webhooks=webhooks;
   if(deliveries){this.webhookDeliveries=deliveries.data||[];this.webhookDeliveryTotal=deliveries.total||0;}
   if(stats)this.webhookDeliveryStats={total:stats.total||0,...(stats.byStatus||{})};
 },
 async loadPageData(pageName=this.page,_force=false){
   try{
     const sessionParam=this.overviewSessionId?'?sessionId='+encodeURIComponent(this.overviewSessionId):'';
     if(pageName==='dashboard'){
       const [dashboard,sessions]=await Promise.all([this.api('/dashboard'+sessionParam),this.api('/sessions')]);
       if(dashboard)this.dashboard=dashboard;if(sessions)this.sessions=sessions;
       await this.loadDashboardMessages();
     } else if(pageName==='sessions'){
       const sessions=await this.api('/sessions');if(sessions)this.sessions=sessions;
     } else if(pageName==='messages'){
       await Promise.all([this.loadMessages(),this.api('/sessions').then(s=>{if(s)this.sessions=s;})]);
     } else if(pageName==='apikeys'){
       const keys=await this.api('/api-keys');if(keys)this.apiKeys=keys;
     } else if(pageName==='templates'){
       const templates=await this.api('/templates');if(templates)this.templates=templates;
     } else if(pageName==='webhooks'){
       await this.loadWebhookData();
     } else if(pageName==='queue'){
       await this.loadQueue();
     } else if(pageName==='stats'){
       const [stats,sessions]=await Promise.all([this.api('/stats'+sessionParam),this.api('/sessions')]);
       if(stats)this.stats=stats;if(sessions)this.sessions=sessions;
     } else if(pageName==='monitoring'){
       const monitoring=await this.api('/monitoring');if(monitoring)this.mon=monitoring;
     }
   }catch{}
 },
 async loadAll(){return this.loadPageData(this.page,true)},
 queuePageItems(){
   const totalPages=Math.max(Math.ceil((this.queue.total||0)/(this.queue.limit||20)),1);
   const current=this.queue.page||1;
   const items=[];
   const pushPage = (value) => items.push({ key: 'page-' + value, type: 'page', value, label: value });
   const pushEllipsis = (side) => items.push({ key: 'ellipsis-' + side + '-' + current + '-' + totalPages, type: 'ellipsis' });

   if (totalPages <= 7) {
     for (let pageNumber = 1; pageNumber <= totalPages; pageNumber++) pushPage(pageNumber);
     return items;
   }

   pushPage(1);
   const left = Math.max(current - 1, 2);
   const right = Math.min(current + 1, totalPages - 1);

   if (left > 2) pushEllipsis('left');
   for (let pageNumber = left; pageNumber <= right; pageNumber++) pushPage(pageNumber);
   if (right < totalPages - 1) pushEllipsis('right');
   pushPage(totalPages);
   return items;
 },
 async goToQueuePage(pageNumber){
   if (pageNumber === '...') return;
   const totalPages=Math.max(Math.ceil((this.queue.total||0)/(this.queue.limit||20)),1);
   const nextPage=Math.min(Math.max(Number(pageNumber)||1,1), totalPages);
   if (nextPage === this.queue.page) return;
   this.queue.page=nextPage;
   await this.loadAll();
 },
 async changeQueuePage(delta){
   const nextPage=Math.max((this.queue.page||1)+delta,1);
   const maxPage=Math.max(Math.ceil((this.queue.total||0)/(this.queue.limit||20)),1);
   if(nextPage===this.queue.page || nextPage>maxPage) return;
   this.queue.page=nextPage;
   await this.loadAll();
 },
 statMaxDaily(){
   const series = this.stats?.dailyBreakdown || [];
   const values = series.map(item => Number(item.total) || 0);
   return Math.max(1, ...values);
 },
 statCount(value){
   return Number(value) || 0;
 },
 statDayLabel(value){
   if (!value) return '-';
   return new Date(value).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' });
 },
 qrSeconds(session){
   this.qrTick;
   if(!session?.qr_expires_at)return 0;
   return Math.max(0, Math.ceil((new Date(session.qr_expires_at).getTime()-Date.now())/1000));
 },
 async login(){const res=await fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'include',body:JSON.stringify({username:this.username,password:this.password})});if(res.ok){const d=await res.json();await this.persistTokens(d);this.error='';await this.loadAll()}else{this.error='Invalid'}},
 async logout(){
   try {
     if (this.token) {
        await fetch('/api/v1/auth/logout', {
          method: 'POST',
          headers: { Authorization: 'Bearer ' + this.token },
          credentials: 'include',
        });
      }
   } catch {}
   this.clearTimers();
   this.token = '';
   localStorage.removeItem('token');
   localStorage.removeItem('refreshToken');
 },
}}
