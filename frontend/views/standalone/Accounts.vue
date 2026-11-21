<template>
  <section class="management-page" :class="{'accounts-page':mode==='accounts','pill-bar':mode==='roles'}">

    <div v-if="mode==='password'" class="pw-page">
      <section class="pw-card profile-card">
        <header class="pw-head">
          <span class="pw-badge" aria-hidden="true"><svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="3.6"/><path d="M5 20c.6-3.7 3.4-5.6 7-5.6s6.4 1.9 7 5.6"/></svg></span>
          <div>
            <h2>个人资料</h2>
            <p>头像会显示在页面右上角。</p>
          </div>
        </header>
        <div class="profile-avatar-row">
          <el-avatar :size="88" :src="user.avatar||'/avatar'" class="profile-avatar"><el-icon :size="36"><User /></el-icon></el-avatar>
          <div class="profile-avatar-actions">
            <input ref="avatarInput" type="file" accept="image/png,image/jpeg,image/webp" hidden @change="onAvatarPicked" />
            <el-button type="primary" :loading="avatarSaving" @click="avatarInput?.click()">上传头像</el-button>
            <el-button v-if="user.avatar" :disabled="avatarSaving" @click="clearAvatar">恢复默认</el-button>
            <p class="pw-note">支持 JPG / PNG / WebP，会自动裁成正方形。</p>
          </div>
        </div>
        <dl class="profile-list">
          <div><dt>账号</dt><dd>{{profile.username||user.username||'—'}}</dd></div>
          <div><dt>身份</dt><dd>{{profile.role==='admin'?'管理员':'子账号'}}</dd></div>
          <div v-if="profile.roles?.length"><dt>角色</dt><dd>{{profile.roles.join('、')}}</dd></div>
          <div v-if="profile.employeeName"><dt>关联员工</dt><dd>{{profile.employeeName}}<span v-if="profile.employeeCode" class="muted"> · {{profile.employeeCode}}</span></dd></div>
          <div v-if="profile.position"><dt>岗位</dt><dd>{{profile.position}}</dd></div>
          <div v-if="profile.lastLoginAt"><dt>最近登录</dt><dd>{{new Date(profile.lastLoginAt).toLocaleString()}}</dd></div>
        </dl>
      </section>
      <section class="pw-card">
        <header class="pw-head">
          <span class="pw-badge" aria-hidden="true"><svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="4.5" y="10.5" width="15" height="10" rx="2.5"/><path d="M8 10.5V8a4 4 0 0 1 8 0v2.5"/><circle cx="12" cy="15.5" r="1.2" fill="currentColor" stroke="none"/></svg></span>
          <div>
            <h2>修改登录密码</h2>
            <p>保存后会退出所有设备，需要用新密码重新登录。</p>
          </div>
        </header>
        <el-form class="pw-form" label-position="top" @submit.prevent="changePassword">
          <el-form-item label="当前密码">
            <el-input v-model="password.currentPassword" type="password" show-password size="large" placeholder="输入现在使用的密码" autocomplete="current-password" />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input v-model="password.password" type="password" show-password size="large" placeholder="至少 6 位" autocomplete="new-password" />
            <div class="pw-meter" :class="'lv-'+pwStrength.level" aria-live="polite">
              <div class="pw-bars" aria-hidden="true"><i></i><i></i><i></i><i></i></div>
              <span class="pw-meter-text" :class="{bad:pwStrength.bad}">{{pwStrength.label}}</span>
              <span class="pw-bytes" :class="{bad:pwBytes>72}">{{pwBytes}} / 72 字节</span>
            </div>
          </el-form-item>
          <el-form-item label="确认新密码">
            <el-input v-model="password.confirm" type="password" show-password size="large" placeholder="再输入一次新密码" autocomplete="new-password" />
            <p v-if="password.confirm" class="pw-match" :class="pwMatch?'ok':'bad'" aria-live="polite">{{pwMatch?'两次输入一致':'两次输入不一致'}}</p>
          </el-form-item>
          <el-button class="pw-submit" type="primary" native-type="submit" size="large" :loading="saving">保存密码并重新登录</el-button>
        </el-form>
      </section>
    </div>
    <section v-else-if="mode==='memory'" class="panel resource-panel">
      <div class="panel-toolbar"><h3>容量监控与安全清理</h3><div><el-button @click="loadMemory">刷新</el-button><el-button v-permission="'maintenance:edit'" @click="quotaVisible=true">⚙ 设置</el-button><el-button v-permission="'maintenance:clean'" type="primary" :loading="saving" @click="cleanMemory">手动安全清理</el-button></div></div>
      <el-alert v-for="alert in visibleAlerts(memory.snapshot?.alerts)" :key="alert" :title="alert" type="warning" :closable="false" show-icon class="resource-alert"/>
      <div class="resource-metrics">
        <article><span>服务器磁盘</span><strong>{{memory.snapshot?.diskTotal ? memory.snapshot.diskPercent.toFixed(1)+'%' : '监控不可用'}}</strong><el-progress :percentage="Math.min(100,memory.snapshot?.diskPercent||0)" :status="memory.snapshot?.diskPercent>80?'exception':undefined"/><small>可用 {{bytes(memory.snapshot?.diskAvailable)}} / 总计 {{bytes(memory.snapshot?.diskTotal)}}</small></article>
        <article><span>当前外部数据库</span><strong>{{memory.snapshot?.databasePercent == null ? '待配置额度' : memory.snapshot.databasePercent.toFixed(1)+'%'}}</strong><el-progress :percentage="Math.min(100,memory.snapshot?.databasePercent||0)" :status="memory.snapshot?.databasePercent>80?'exception':undefined"/><small>已用 {{bytes(memory.snapshot?.databaseBytes)}}，按本数据库额度统计</small></article>
        <article><span>服务内存</span><strong>{{bytes(memory.heapBytes)}}</strong><small>运行时保留 {{bytes(memory.systemBytes)}}；清理时尝试归还空闲内存</small></article>
      </div>
      <div class="policy-head">
        <div><h3>保留策略</h3><p class="muted">每分钟监测，超过 80% 触发自动清理，执行间隔至少 30 分钟；也可点击右上角“手动安全清理”。账号、数据库底层文件不在清理范围。</p></div>
        <el-button v-permission="'maintenance:edit'" type="primary" :loading="saving" @click="savePolicy">保存策略</el-button>
      </div>
      <el-form label-position="top" class="policy-board" :disabled="!user.hasPermission('maintenance:edit') || saving" @submit.prevent>
        <div class="policy-top">
        <section class="policy-card pc-auto">
          <header class="pc-head">
            <span class="pc-ico">⚡</span>
            <div class="pc-title"><b>自动清理</b><small>开启后，容量超过 80% 时自动执行安全清理</small></div>
            <el-switch v-model="policy.auto" inline-prompt active-text="开" inactive-text="关"/>
          </header>
          <div class="pc-field">
            <label>数据库容量额度</label>
            <small>填写服务商分配给本数据库的容量；0 表示未知，无法判断 80% 阈值</small>
            <div class="unit-input"><el-input-number v-model="quotaGiB" :min="0" :precision="3"/><em>GiB</em></div>
          </div>
        </section>
        <section class="policy-card pc-run">
          <header class="pc-head">
            <span class="pc-ico">🧹</span>
            <div class="pc-title"><b>运行数据</b><small>过期会话、验证码、绑定码会直接清理；下列记录按天数保留</small></div>
            <span class="pc-badge safe">安全</span>
          </header>
          <div class="pc-grid">
            <div v-for="f in runFields" :key="f.key" class="pc-field">
              <label>{{f.label}}<el-popover trigger="click" placement="top" :width="240" :content="f.hint"><template #reference><button type="button" class="hint-btn" aria-label="查看说明">?</button></template></el-popover></label>
              <div class="unit-input"><el-input-number v-model="policy[f.key]" :min="f.min" :max="f.max"/><em>{{f.unit}}</em></div>
            </div>
          </div>
        </section>
        </div>
        <section class="policy-card keep-card">
          <header class="pc-head">
            <span class="pc-ico">🗂</span>
            <div class="pc-title"><b>业务数据</b><small>默认永久保留；设置保留期后，到期记录将被永久删除且不可恢复</small></div>
            <span class="pc-badge warn">谨慎</span>
          </header>
          <div class="keep-list">
            <div v-for="it in keepItems" :key="it.key" class="keep-row" :class="{on:policy[it.key]>0}">
              <div class="keep-info"><b>{{it.label}}</b><small>{{it.hint}}</small></div>
              <div class="keep-ctl">
                <el-radio-group :model-value="policy[it.key]>0?'limit':'forever'" size="small" @update:model-value="setKeep(it,$event)">
                  <el-radio-button label="forever">永久保留</el-radio-button>
                  <el-radio-button label="limit">定期清理</el-radio-button>
                </el-radio-group>
                <div v-if="policy[it.key]>0" class="unit-input sm"><el-input-number v-model="policy[it.key]" :min="3" :max="120" size="small"/><em>个月</em></div>
              </div>
            </div>
          </div>
          <p class="keep-note">定期清理的范围为 3～120 个月。归档表删除后，对应日期的单条打卡明细将无法查看或导出；罚款记录删除后，罚款详情中的历史罚款也会消失；导出备份删除后无法再从“备份”列表下载。</p>
        </section>
      </el-form>
      <p class="muted">数据库额度请填写服务商分配给本数据库的容量，不代表服务商整台磁盘使用率。安全 VACUUM 回收可复用空间并尝试释放空闲尾页；不会自动执行需要锁表和额外空间的 FULL。下表为执行前后观测差值，可能受并发写入影响，不将删除行数当作磁盘释放量。</p>
      <h3>执行日志</h3><el-table :data="memory.history||[]" empty-text="尚无清理记录">
        <el-table-column type="expand"><template #default="{row}"><div class="resource-detail"><p v-for="(step,i) in row.steps" :key="i">{{step}}</p><p v-for="(error,i) in row.errors" :key="'error'+i" class="resource-error">{{error}}</p><p v-for="(alert,i) in visibleAlerts(row.after?.alerts)" :key="'alert'+i" class="resource-error">{{alert}}</p></div></template></el-table-column>
        <el-table-column label="执行时间" min-width="180"><template #default="{row}">{{new Date(row.startedAt).toLocaleString()}}</template></el-table-column>
        <el-table-column label="触发方式" width="100"><template #default="{row}">{{row.trigger==='automatic'?'自动':'手动'}}</template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="{row}"><el-tag :type="row.status==='completed'?'success':'warning'">{{runStatus[row.status]||row.status}}</el-tag></template></el-table-column>
        <el-table-column prop="deletedRows" label="清理行数" width="100"/>
        <el-table-column label="磁盘可用增量" min-width="130"><template #default="{row}">{{bytes(row.observedDiskReleased)}}</template></el-table-column>
        <el-table-column label="数据库体积减少" min-width="140"><template #default="{row}">{{bytes(row.observedDatabaseReleased)}}</template></el-table-column>
      </el-table>
      <el-pagination class="list-pagination" background layout="total, prev, pager, next" :total="memory.total||0" :page-size="10" :current-page="memoryPage" :disabled="memoryLoading||saving" @current-change="changeMemoryPage"/>
    </section>
    <section v-else-if="mode==='roles'" class="panel roles-panel">
      <header class="roles-head">
        <h3>角色与权限</h3>
        <div class="roles-actions">
          <el-input v-model="roleSearch" class="roles-search" clearable placeholder="搜索角色名称或描述"/>
          <el-button @click="loadRoles" :loading="loading">刷新</el-button>
          <el-button v-permission="'roles:edit'" type="primary" @click="editRole()">＋ 创建角色</el-button>
        </div>
      </header>
      <el-table :data="filteredRoles" v-loading="loading" class="roles-table" empty-text="暂无角色，请创建一个角色">
        <el-table-column label="角色" min-width="240">
          <template #default="{row}">
            <div class="role-cell">
              <span class="role-avatar" :class="roleKind(row).key">{{[...row.name][0]}}</span>
              <div class="role-main">
                <div class="role-name"><b>{{row.name}}</b><el-tag size="small" :type="roleKind(row).type" effect="light">{{roleKind(row).text}}</el-tag></div>
                <small class="role-desc" :title="row.description">{{row.description||'暂无描述'}}</small>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="关联账号" min-width="170">
          <template #default="{row}">
            <div class="role-members">
              <el-tooltip v-if="row.userCount" placement="top" :show-after="150">
                <template #content>
                  <div class="role-member-tip">{{row.members.join('、')}}{{row.userCount>row.members.length?' 等 '+row.userCount+' 个账号':''}}<br/><small>点击查看账号详情</small></div>
                </template>
                <button type="button" class="role-member-count" @click="openRoleMembers(row)">{{row.userCount}}</button>
              </el-tooltip>
              <b v-else class="zero">0</b>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="权限概览" min-width="320">
          <template #default="{row}">
            <el-tag v-if="row.protected" type="success" effect="light">全部权限</el-tag>
            <div v-else-if="rolePermGroups(row).length" class="role-perms">
              <el-tooltip v-for="g in rolePermGroups(row).slice(0,5)" :key="g.key" :content="g.actions.join('、')" placement="top"><span class="role-chip">{{g.label}}<em>{{g.actions.length}}</em></span></el-tooltip>
              <el-tooltip v-if="rolePermGroups(row).length>5" :content="rolePermGroups(row).slice(5).map(g=>g.label).join('、')" placement="top"><span class="role-chip more">+{{rolePermGroups(row).length-5}}</span></el-tooltip>
              <small class="role-total">共 {{row.permissions.length}} 项</small>
            </div>
            <span v-else class="role-none">未授权：无业务页面访问权限</span>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="150"><template #default="{row}"><span class="role-time">{{fmtTime(row.createdAt)}}</span></template></el-table-column>
        <el-table-column label="更新时间" width="150"><template #default="{row}"><span class="role-time">{{fmtTime(row.updatedAt)}}</span></template></el-table-column>
        <el-table-column label="操作" width="130" fixed="right"><template #default="{row}"><el-button v-permission="'roles:edit'" link type="primary" :disabled="row.protected" @click="editRole(row)">编辑</el-button><el-button v-permission="'roles:delete'" link type="danger" :disabled="row.protected || row.name==='查看员' || row.userCount>0" @click="deleteRole(row)">删除</el-button></template></el-table-column>
      </el-table>
    </section>
    <section v-else class="panel">
      <div class="panel-toolbar accounts-toolbar">
        <h3>员工子账号</h3>
        <div class="accounts-actions">
          <el-input v-model="accountSearch" class="accounts-search" clearable placeholder="搜索账号、员工姓名、工号或 TG ID" @keyup.enter="searchAccounts">
            <template #prefix><svg class="search-icon" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg></template>
          </el-input>
          <el-button class="accounts-batch" @click="openBatch">批量操作<span v-if="selected.length" class="batch-count">{{selected.length}}</span></el-button>
          <el-button v-permission="'accounts:edit'" class="accounts-add" type="primary" @click="openCreate"><span class="plus">＋</span>新增子账号</el-button>
          <div class="global-totp"><span>全局谷歌验证</span><el-switch :model-value="globalTotp" :loading="securitySaving" :disabled="!user.hasPermission('accounts:security')" @change="value=>setLoginSecurity(null,!!value)"/></div>
        </div>
      </div>
      <el-table ref="tableRef" :data="rows" row-key="id" v-loading="loading" @selection-change="onSelect"><el-table-column type="selection" width="48" :reserve-selection="true"/><el-table-column prop="username" label="账号" min-width="130" class-name="big-col"/><el-table-column prop="name" label="员工" min-width="110" class-name="big-col"/><el-table-column label="权限" min-width="110"><template #default="{row}"><el-tag :type="permissionTag(row).type" effect="light">{{permissionTag(row).text}}</el-tag></template></el-table-column><el-table-column label="TG ID" min-width="150" class-name="tg-col"><template #default="{row}"><template v-if="row.telegramUserId"><el-tooltip v-if="user.hasPermission('accounts:edit')" content="点击更改绑定" placement="top"><el-button link @click="binding(row)">{{row.telegramUserId}}</el-button></el-tooltip><span v-else>{{row.telegramUserId}}</span></template><el-button v-else-if="user.hasPermission('accounts:edit')" link @click="binding(row)">绑定 TG</el-button><span v-else class="muted">未绑定</span></template></el-table-column>
        <el-table-column label="在线状态" min-width="175"><template #header><el-tooltip content="在线：最近 2 分钟内后台有活动且登录会话有效；否则显示最后登录时间"><span>在线状态 ⓘ</span></el-tooltip></template><template #default="{row}"><el-tag v-if="row.online" round type="success">在线</el-tag><span v-else-if="row.lastLoginAt">{{new Date(row.lastLoginAt).toLocaleString()}}</span><span v-else class="muted">从未登录</span></template></el-table-column>
        <el-table-column label="谷歌验证" min-width="180"><template #default="{row}"><div class="totp-cell"><el-switch :model-value="row.totpRequired" :disabled="securitySaving||!user.hasPermission('accounts:security')" @change="value=>setLoginSecurity(row,!!value)"/><span>{{globalTotp?'全局强制':row.totpRequired?'个人开启':'普通验证码'}}</span><small v-if="globalTotp||row.totpRequired">{{row.totpBound?'已绑定':'下次登录绑定'}}</small></div></template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="{row}"><el-switch :model-value="row.status===1" inline-prompt active-text="启用" inactive-text="禁用" :loading="pendingAccounts.has(row.id)" :disabled="!user.hasPermission('accounts:edit')" @change="toggle(row)"/></template></el-table-column>
        <el-table-column label="操作" min-width="190" class-name="op-col"><template #default="{row}">
          <div class="row-ops">
          <el-tooltip v-if="user.hasPermission('accounts:security')" :content="row.totpBound?'重置谷歌验证码':'绑定谷歌验证码'" placement="top"><el-button class="icon-btn" circle :type="row.totpBound?'warning':'primary'" :disabled="securitySaving" :aria-label="row.totpBound?'重置谷歌验证码':'绑定谷歌验证码'" @click="row.totpBound?setLoginSecurity(row,row.totpRequired,true):bindGoogle(row)"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.2"/><rect x="14" y="3" width="7" height="7" rx="1.2"/><rect x="3" y="14" width="7" height="7" rx="1.2"/><path d="M14 14h3v3M21 14v.01M14 21h3M21 17v4h-4"/></svg></el-button></el-tooltip>
          <el-tooltip v-if="user.hasPermission('roles:assign')||user.hasPermission('accounts:reset_password')" content="编辑（角色 / 密码）" placement="top"><el-button class="icon-btn" circle aria-label="编辑" @click="openEdit(row)"><el-icon><Edit /></el-icon></el-button></el-tooltip>
          <el-tooltip v-if="user.hasPermission('accounts:reset')" content="重置打卡" placement="top"><el-button class="icon-btn" circle type="warning" :loading="pendingAccounts.has(row.id)" aria-label="重置打卡" @click="resetAttendance(row)"><el-icon><RefreshLeft /></el-icon></el-button></el-tooltip>
          <el-tooltip v-if="user.hasPermission('accounts:delete')" content="删除" placement="top"><el-button class="del-btn icon-btn" circle :loading="pendingAccounts.has(row.id)" aria-label="删除" @click="deleteAccount(row)"><el-icon><Delete /></el-icon></el-button></el-tooltip>
          </div>
        </template></el-table-column>
      </el-table>
      <el-pagination class="list-pagination" background layout="total, prev, pager, next, jumper" :total="accountTotal" :page-size="10" :current-page="accountPage" :disabled="loading" @current-change="changeAccountPage"/>
    </section>
    <el-dialog v-model="quotaVisible" title="外部数据库配额设置" width="460px"><el-form label-position="top" @submit.prevent="saveQuota"><el-form-item label="数据库容量额度（GiB；0 表示未知）"><el-input-number v-model="quotaGiB" :min="0" :precision="3" style="width:100%"/></el-form-item><p class="muted">请填写服务商分配给本数据库的容量（例如 5 GiB），用于判断数据库使用率是否超过 80%。</p></el-form><template #footer><el-button @click="quotaVisible=false">取消</el-button><el-button type="primary" :loading="saving" v-permission="'maintenance:edit'" @click="saveQuota">保存</el-button></template></el-dialog>
    <el-drawer v-model="memberVisible" :title="'关联账号 · '+(memberRole?.name||'')" size="480px" append-to-body modal-class="cool-drawer-mask" class="cool-drawer pill-bar">
      <div v-loading="memberLoading" class="member-list">
        <div class="member-summary">共 <b>{{memberItems.length}}</b> 个账号关联此角色</div>
        <el-empty v-if="!memberLoading&&!memberItems.length" description="暂无关联账号" />
        <div v-for="m in memberItems" :key="m.id" class="member-card" :class="{disabled:m.status!==1}">
          <span class="member-avatar">{{[...(m.name||m.username)][0]}}<i v-if="m.online" class="member-online"></i></span>
          <div class="member-main">
            <div class="member-title"><b>{{m.username}}</b><el-tag v-if="m.status!==1" size="small" type="danger" effect="light">已禁用</el-tag><el-tag v-else-if="m.online" size="small" type="success" effect="light">在线</el-tag></div>
            <div class="member-meta"><span>姓名：{{m.name||'-'}}</span><span v-if="m.employeeCode">工号：{{m.employeeCode}}</span></div>
            <div class="member-meta"><span v-if="m.telegramUserId">TG ID：{{m.telegramUserId}}</span><span>最后登录：{{m.lastLoginAt?fmtTime(m.lastLoginAt):'从未登录'}}</span></div>
          </div>
        </div>
      </div>
      <template #footer><el-button type="primary" @click="memberVisible=false">关闭</el-button></template>
    </el-drawer>
    <el-drawer v-model="roleVisible" :title="roleForm.id?'编辑角色':'创建角色'" size="min(960px, 95vw)" append-to-body modal-class="cool-drawer-mask" class="cool-drawer pill-bar">
      <el-form label-position="top" class="role-form"><el-form-item label="角色名称"><el-input v-model="roleForm.name" maxlength="100" show-word-limit :disabled="!!roleForm.id && roleForm.name==='查看员'"/></el-form-item><el-form-item label="角色描述（选填）"><el-input v-model="roleForm.description" type="textarea" :rows="2" maxlength="200" show-word-limit placeholder="例如：负责考勤审核，可查看并导出打卡记录"/></el-form-item></el-form>
      <el-alert :closable="false" type="info" title="编辑自动包含查看；删除、审核、审批、导出等操作需单独勾选。全员数据权限决定是否能管理其他员工。"/>
      <div class="permission-summary"><span>已选择 <b>{{roleForm.permissions.length}}</b> / {{totalGrantable}} 项权限</span><span><el-button link type="primary" @click="grantAll(true)">全选</el-button><el-button link @click="grantAll(false)">清空</el-button></span></div>
      <div class="permission-matrix"><div v-for="group in catalog" :key="group.key" class="permission-row"><strong>{{group.label}}<small>{{groupCount(group)}} / {{group.actions.length}}</small></strong><div><el-checkbox v-for="action in group.actions" :key="action" :model-value="roleForm.permissions.includes(group.key+':'+action)" :disabled="!user.hasPermission(group.key+':'+action) || (action==='view' && roleForm.permissions.includes(group.key+':edit'))" @change="value=>setGrant(group.key,action,!!value)">{{group.key==='punch' && action==='edit' ? '打卡' : actionLabels[action]||action}}</el-checkbox></div></div></div>
      <template #footer><el-button @click="roleVisible=false">取消</el-button><el-button type="primary" :loading="saving" v-permission="'roles:edit'" @click="saveRole">保存角色</el-button></template>
    </el-drawer>
    <el-dialog class="pill-dialog" v-model="editVisible" :title="'编辑账号 · '+(editTarget?.username||'')" width="480px" append-to-body>
      <el-form label-position="top" @submit.prevent="saveEdit">
        <el-form-item v-if="user.hasPermission('roles:assign')" label="角色">
          <el-select v-model="editRoles" multiple placeholder="选择角色" style="width:100%"><el-option v-for="r in roles.filter(x=>!x.protected)" :key="r.id" :value="r.id" :label="r.name"/></el-select>
          <p class="muted edit-tip">清空角色后，该账号将无法访问业务菜单和接口。</p>
        </el-form-item>
        <el-form-item v-if="user.hasPermission('accounts:reset_password')" label="新密码（不修改请留空）">
          <el-input v-model="editPassword" type="password" show-password placeholder="6–72 字节" autocomplete="new-password"/>
          <p class="muted edit-tip">修改密码后，该账号现有登录会话会退出。</p>
        </el-form-item>
      </el-form>
      <template #footer><el-button @click="editVisible=false">取消</el-button><el-button type="primary" :loading="saving" @click="saveEdit">保存</el-button></template>
    </el-dialog>
    <el-dialog class="pill-dialog" v-model="batchVisible" :title="'批量操作 · 已选 '+selected.length+' 个账号'" width="520px" append-to-body>
      <el-radio-group v-model="batchType" class="batch-types">
        <el-radio-button v-for="t in batchTypes" :key="t.key" :value="t.key" :disabled="!user.hasPermission(t.perm)">{{t.label}}</el-radio-button>
      </el-radio-group>
      <div class="batch-body">
        <template v-if="batchType==='roles'">
          <el-select v-model="batchRoles" multiple placeholder="选择角色" style="width:100%"><el-option v-for="r in roles.filter(x=>!x.protected)" :key="r.id" :value="r.id" :label="r.name"/></el-select>
          <p class="muted edit-tip">所选账号原有角色会被替换为这里选择的角色；不选则清空角色，账号将无法访问业务菜单和接口。</p>
        </template>
        <template v-else-if="batchType==='password'">
          <el-input v-model="batchPassword" type="password" show-password placeholder="新密码（6–72 字节）" autocomplete="new-password"/>
          <p class="muted edit-tip">所选账号都会改成这个密码，旧登录会话会退出。</p>
        </template>
        <template v-else-if="batchType==='reset'">
          <el-radio-group v-model="batchResetMode" class="batch-reset"><el-radio value="all">重置上下班（上班、下班与活动次数全部重新开始）</el-radio><el-radio value="activity">只重置活动次数</el-radio></el-radio-group>
          <p class="muted edit-tip">仅影响当前业务日，原记录归档保留。</p>
        </template>
        <template v-else>
          <el-alert type="error" :closable="false" show-icon title="将永久删除所选子账号及其对应员工，考勤明细中的该员工、TG 绑定、角色和登录会话会一并删除，不可恢复。"/>
        </template>
        <div class="batch-names">{{selected.map(r=>r.username).join('、')}}</div>
      </div>
      <template #footer><el-button @click="batchVisible=false">取消</el-button><el-button :class="batchType==='delete'?'del-btn':''" :type="batchType==='delete'?'default':'primary'" :loading="batchRunning" @click="runBatch">{{batchType==='delete'?'确认删除':'确认执行'}}</el-button></template>
    </el-dialog>
    <el-drawer v-model="visible" title="新增子账号" size="460px" append-to-body modal-class="cool-drawer-mask" class="cool-drawer"><el-form label-position="top"><el-form-item label="关联员工"><el-select v-model="form.employeeId" filterable style="width:100%"><el-option v-for="e in employees" :key="e.id" :value="e.id" :label="e.name"/></el-select></el-form-item><el-form-item label="账号（3–64位字母、数字或_.-）"><el-input v-model="form.username"/></el-form-item><el-form-item label="初始密码（至少6位）"><el-input v-model="form.password" type="password" show-password/></el-form-item></el-form><template #footer><el-button @click="visible=false">取消</el-button><el-button type="primary" :loading="saving" v-permission="'accounts:edit'" @click="create">创建</el-button></template></el-drawer>
    <el-dialog class="pill-dialog" v-model="bindVisible" :title="'绑定 Telegram · '+bindTarget?.username" width="480px"><el-form label-position="top" @submit.prevent="saveBinding"><el-form-item label="Telegram 用户 ID"><el-input v-model="telegramId" inputmode="numeric" placeholder="请输入数字 TG ID" maxlength="16"/></el-form-item><p class="muted">保存后替换该员工原有的 TG 绑定；不能使用其他员工已绑定的 ID。</p><el-button type="primary" native-type="submit" :loading="saving">保存绑定</el-button></el-form></el-dialog>
  </section>
</template>
<script setup lang="ts">
import {ref,computed,watch,onMounted,onUnmounted} from 'vue';
import {useRoute} from 'vue-router';
import {ElMessage,ElMessageBox} from 'element-plus';
// 弹窗统一加 pill-dialog 圆润样式
const box={confirm:(m:any,t:any,o:any={})=>ElMessageBox.confirm(m,t,{customClass:'pill-dialog',...o}),prompt:(m:any,t:any,o:any={})=>ElMessageBox.prompt(m,t,{customClass:'pill-dialog',...o}),alert:(m:any,t:any,o:any={})=>ElMessageBox.alert(m,t,{customClass:'pill-dialog',...o})};
import request from '../../api/request';
import {useUserStore} from '../../store/user';
import {User,Edit,Delete,RefreshLeft} from '@element-plus/icons-vue';
import {notifyEmployeesChanged} from '../../api/employeeSync';
const user=useUserStore(),route=useRoute();
const mode=computed(()=>route.path.slice(1));
const rows=ref<any[]>([]),roles=ref<any[]>([]),catalog=ref<any[]>([]),employees=ref<any[]>([]),loading=ref(false),saving=ref(false),visible=ref(false),bindVisible=ref(false),form=ref({username:'',password:'',employeeId:''});
const roleVisible=ref(false),assignVisible=ref(false),target=ref<any>(null),assigned=ref<string[]>([]),roleForm=ref({id:'',name:'',description:'',permissions:[] as string[]}),roleSearch=ref(''),password=ref({currentPassword:'',password:'',confirm:''}),memory=ref<any>({});
const pwBytes=computed(()=>new TextEncoder().encode(password.value.password).length);
const profile=ref<any>({}),avatarInput=ref<HTMLInputElement|null>(null),avatarSaving=ref(false);
async function loadProfile(){try{const r:any=await request.get('/auth/profile');profile.value=r.data||{};if(typeof r.data?.avatar==='string')user.avatar=r.data.avatar}catch(e){report(e)}}
function cropAvatar(file:File):Promise<string>{return new Promise((resolve,reject)=>{const url=URL.createObjectURL(file),img=new Image();img.onload=()=>{try{const size=256,side=Math.min(img.width,img.height),canvas=document.createElement('canvas');canvas.width=canvas.height=size;const ctx=canvas.getContext('2d');if(!ctx)throw new Error('浏览器不支持图片处理');ctx.fillStyle='#fff';ctx.fillRect(0,0,size,size);ctx.drawImage(img,(img.width-side)/2,(img.height-side)/2,side,side,0,0,size,size);resolve(canvas.toDataURL('image/jpeg',0.86))}catch(e){reject(e)}finally{URL.revokeObjectURL(url)}};img.onerror=()=>{URL.revokeObjectURL(url);reject(new Error('无法读取这张图片'))};img.src=url})}
async function saveAvatar(avatar:string){avatarSaving.value=true;try{const r:any=await request.post('/auth/avatar',{avatar});user.avatar=r.data?.avatar??avatar;ElMessage.success(avatar?'头像已更新':'已恢复默认头像')}catch(e){report(e)}finally{avatarSaving.value=false}}
async function onAvatarPicked(ev:Event){const input=ev.target as HTMLInputElement,file=input.files?.[0];input.value='';if(!file)return;if(!/^image\/(png|jpeg|webp)$/.test(file.type)){ElMessage.warning('仅支持 JPG / PNG / WebP 图片');return}if(file.size>10*1024*1024){ElMessage.warning('图片不能超过 10MB');return}try{await saveAvatar(await cropAvatar(file))}catch(e){report(e)}}
async function clearAvatar(){await saveAvatar('')}
const pwLenOk=computed(()=>pwBytes.value>=6&&pwBytes.value<=72);
const pwMatch=computed(()=>!!password.value.confirm&&password.value.confirm===password.value.password);
const pwStrength=computed(()=>{const v=password.value.password,n=pwBytes.value;if(!v)return{level:0,bad:false,label:'输入后显示强度'};if(n<6)return{level:0,bad:true,label:'太短，至少 6 字节'};if(n>72)return{level:0,bad:true,label:'超过 72 字节'};const len=[...v].length;let sc=0;if(len>=8)sc++;if(len>=12)sc++;if((/[a-z]/.test(v)&&/[A-Z]/.test(v))||(/\d/.test(v)&&/[A-Za-z]/.test(v)))sc++;if(/[^A-Za-z0-9]/.test(v))sc++;const level=Math.max(1,sc);return{level,bad:false,label:['','强度较弱','强度一般','强度较强','强度很强'][level]}});
const actionLabels:Record<string,string>={view:'查看',edit:'编辑',delete:'删除',review:'审核',approve:'审批',export:'导出',assign:'分配角色',reset_password:'重置密码',reset:'重置打卡',notify:'发送通知',clean:'执行清理',all:'管理全员数据',security:'登录验证'};
function validPassword(v:string){const n=new TextEncoder().encode(v).length;return n>=6&&n<=72}
function report(e:any){if(e!=='cancel'&&e!=='close')ElMessage.error(e.message||'操作失败')}
const globalTotp=ref(false),securitySaving=ref(false);
async function loadLoginPolicy(){const r:any=await request.get('/standalone/login-policy');globalTotp.value=r.data.global}
async function setLoginSecurity(row:any,enabled:boolean,reset=false){securitySaving.value=true;try{if(reset&&row){await box.confirm('重置后，'+row.username+' 下次登录需要重新绑定谷歌验证器，其登录会话将退出。','重置谷歌验证码',{type:'warning',confirmButtonText:'重置',cancelButtonText:'取消',closeOnClickModal:false});await request.put('/standalone/accounts/'+row.id+'/totp',{enabled:row.totpRequired,reset:true});await Promise.all([load(),loadLoginPolicy()]);ElMessage.success('谷歌验证码已重置');return}const text=reset?'重置后，该用户下次需要重新绑定验证器。':row?(enabled?'开启该用户谷歌验证。':'关闭个人开关；全局开启时仍需谷歌验证。'):(enabled?'管理员及全部子账号都将使用谷歌验证。':'关闭全局强制；个人已开启的用户仍需谷歌验证。');const {value}=await box.prompt(text+' 其他受影响的登录会话将退出。请输入你当前的登录密码确认。',reset?'重置谷歌绑定':'登录验证设置',{inputType:'password',inputValidator:v=>!!v||'请输入当前密码',closeOnClickModal:false});await request.put(row?'/standalone/accounts/'+row.id+'/totp':'/standalone/login-policy',{enabled,password:value,reset});await Promise.all([load(),loadLoginPolicy()]);ElMessage.success('登录验证设置已更新')}catch(e){report(e)}finally{securitySaving.value=false}}
const accountSearch=ref(''),appliedSearch=ref(''),accountPage=ref(1),accountTotal=ref(0);
let searchTimer=0;
async function searchAccounts(){window.clearTimeout(searchTimer);appliedSearch.value=(accountSearch.value||'').trim();accountPage.value=1;try{await load()}catch(e){report(e)}}
watch(accountSearch,()=>{if(mode.value!=='accounts')return;window.clearTimeout(searchTimer);searchTimer=window.setTimeout(()=>{void searchAccounts()},300)});
async function changeAccountPage(page:number){accountPage.value=page;try{await load()}catch(e){report(e)}}
let loadSeq=0;
function permissionTag(row:any):{text:string,type:'success'|'info'|'warning'|'primary'}{const names:string[]=(row.roleNames||[]).map((n:string)=>String(n).trim());if(names.some(n=>['admin','administrator','superadmin','管理员','超级管理员'].includes(n.toLowerCase())))return{text:'管理员',type:'success'};if(names.includes('查看员'))return{text:'查看员',type:'info'};if(names.length)return{text:names.join('、'),type:'primary'};return{text:'未授权',type:'warning'}}
async function load(silent=false){const seq=++loadSeq;if(!silent)loading.value=true;try{const r:any=await request.get('/standalone/accounts',{params:{search:appliedSearch.value,page:accountPage.value,pageSize:10}});if(seq!==loadSeq)return;rows.value=r.data.items;accountPage.value=r.data.page;accountTotal.value=r.data.total}finally{if(seq===loadSeq)loading.value=false}}
const memberVisible=ref(false),memberLoading=ref(false),memberRole=ref<any>(null),memberItems=ref<any[]>([]);
async function openRoleMembers(row:any){memberRole.value=row;memberItems.value=[];memberVisible.value=true;memberLoading.value=true;try{const r:any=await request.get('/standalone/roles/'+row.id+'/members');memberItems.value=r.data.items||[]}catch(e){report(e);memberVisible.value=false}finally{memberLoading.value=false}}
async function loadRoles(){loading.value=true;try{const r:any=await request.get('/standalone/roles');roles.value=r.data.items;catalog.value=r.data.catalog}finally{loading.value=false}}
function editRole(row?:any){roleForm.value={id:row?.id||'',name:row?.name||'',description:row?.description||'',permissions:[...(row?.permissions||[])]};roleVisible.value=true}
const filteredRoles=computed(()=>{const k=roleSearch.value.trim().toLowerCase();return k?roles.value.filter(r=>(r.name||'').toLowerCase().includes(k)||(r.description||'').toLowerCase().includes(k)):roles.value});
function roleKind(r:any):{key:string,text:string,type:'success'|'info'|'primary'}{if(r.protected)return{key:'system',text:'系统',type:'success'};if(r.isDefault)return{key:'default',text:'默认',type:'info'};return{key:'custom',text:'自定义',type:'primary'}}
function rolePermGroups(r:any){return catalog.value.map(g=>({key:g.key,label:g.label,actions:g.actions.filter((a:string)=>(r.permissions||[]).includes(g.key+':'+a)).map((a:string)=>g.key==='punch'&&a==='edit'?'打卡':actionLabels[a]||a)})).filter(g=>g.actions.length)}
function fmtTime(v:any){if(!v)return '-';const d=new Date(v);return Number.isNaN(d.getTime())?'-':d.toLocaleString('zh-CN',{hour12:false,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})}
const totalGrantable=computed(()=>catalog.value.reduce((n,g)=>n+g.actions.length,0));
function groupCount(g:any){return g.actions.filter((a:string)=>roleForm.value.permissions.includes(g.key+':'+a)).length}
function grantAll(on:boolean){if(!on){roleForm.value.permissions=[];return}const set=new Set<string>();for(const g of catalog.value)for(const a of g.actions)if(user.hasPermission(g.key+':'+a))set.add(g.key+':'+a);roleForm.value.permissions=[...set]}
function setGrant(module:string,action:string,checked:boolean){const set=new Set(roleForm.value.permissions),key=module+':'+action;if(checked){set.add(key);if(action==='edit')set.add(module+':view')}else set.delete(key);roleForm.value.permissions=[...set]}
async function saveRole(){saving.value=true;try{const f=roleForm.value;if(f.id)await request.put('/standalone/roles/'+f.id,f);else await request.post('/standalone/roles',f);roleVisible.value=false;await user.validateSession();await loadRoles();ElMessage.success('角色已保存')}catch(e){report(e)}finally{saving.value=false}}
async function deleteRole(row:any){try{await box.confirm('确认删除角色 '+row.name+'？','删除角色');await request.delete('/standalone/roles/'+row.id);await loadRoles()}catch(e){report(e)}}
const editVisible=ref(false),editTarget=ref<any>(null),editRoles=ref<string[]>([]),editPassword=ref('');
async function openEdit(row:any){try{if(user.hasPermission('roles:assign'))await loadRoles();editTarget.value=row;editRoles.value=[...(row.roleIds||[])];editPassword.value='';editVisible.value=true}catch(e){report(e)}}
async function saveEdit(){const row=editTarget.value;if(!row)return;const canRoles=user.hasPermission('roles:assign'),canPw=user.hasPermission('accounts:reset_password'),pw=editPassword.value;if(canPw&&pw&&!validPassword(pw)){ElMessage.warning('密码需要6–72字节');return}const rolesChanged=canRoles&&JSON.stringify([...editRoles.value].sort())!==JSON.stringify([...(row.roleIds||[])].sort());if(!rolesChanged&&!(canPw&&pw)){editVisible.value=false;return}saving.value=true;try{if(rolesChanged)await request.put('/standalone/accounts/'+row.id+'/roles',{roleIds:editRoles.value});if(canPw&&pw)await request.post('/standalone/accounts/'+row.id+'/password',{password:pw});editVisible.value=false;if(rolesChanged)await user.validateSession();ElMessage.success('账号已更新')}catch(e){report(e)}finally{saving.value=false;try{await load()}catch{}}}
function bindGoogle(row:any){if(globalTotp.value||row.totpRequired){box.alert(row.username+' 已开启谷歌验证，下次登录时会进入绑定流程，用谷歌验证器扫码即可。','绑定谷歌验证码',{confirmButtonText:'知道了'});return}void setLoginSecurity(row,true)}
const tableRef=ref<any>(null),selected=ref<any[]>([]),batchVisible=ref(false),batchType=ref('roles'),batchRoles=ref<string[]>([]),batchPassword=ref(''),batchResetMode=ref('all'),batchRunning=ref(false);
const batchTypes=[{key:'roles',label:'分配角色',perm:'roles:assign'},{key:'password',label:'重置密码',perm:'accounts:reset_password'},{key:'reset',label:'重置打卡',perm:'accounts:reset'},{key:'delete',label:'删除',perm:'accounts:delete'}];
function onSelect(list:any[]){selected.value=list}
async function openBatch(){if(!selected.value.length){ElMessage.warning('请先勾选要操作的账号');return}const first=batchTypes.find(t=>user.hasPermission(t.perm));if(!first){ElMessage.warning('没有可用的批量操作权限');return}if(!user.hasPermission(batchTypes.find(t=>t.key===batchType.value)!.perm))batchType.value=first.key;batchPassword.value='';batchRoles.value=[];try{if(user.hasPermission('roles:assign'))await loadRoles();batchVisible.value=true}catch(e){report(e)}}
async function runBatch(){const list=[...selected.value],type=batchType.value;if(!list.length)return;if(type==='password'&&!validPassword(batchPassword.value)){ElMessage.warning('密码需要6–72字节');return}
  try{if(type==='delete')await box.confirm('将永久删除选中的 '+list.length+' 个子账号及其对应员工，此操作不可恢复。确认删除？','批量删除',{confirmButtonText:'确认删除',cancelButtonText:'取消',type:'warning',closeOnClickModal:false});else if(type==='roles'&&!batchRoles.value.length)await box.confirm('未选择任何角色，所选账号将被清空角色并无法访问业务页面。继续？','批量分配角色',{type:'warning',closeOnClickModal:false})}catch{return}
  batchRunning.value=true;const failed:string[]=[];let ok=0;
  for(const row of list){try{if(type==='roles')await request.put('/standalone/accounts/'+row.id+'/roles',{roleIds:batchRoles.value});else if(type==='password')await request.post('/standalone/accounts/'+row.id+'/password',{password:batchPassword.value});else if(type==='reset')await request.post('/standalone/accounts/'+row.id+'/reset-daily',{mode:batchResetMode.value});else await request.delete('/standalone/accounts/'+row.id);ok++}catch(e:any){failed.push(row.username+'（'+(e?.message||'失败')+'）')}}
  batchRunning.value=false;batchVisible.value=false;tableRef.value?.clearSelection();selected.value=[];
  if(type==='delete'){if(ok>=rows.value.length&&accountPage.value>1)accountPage.value--;notifyEmployeesChanged('accounts')}
  if(type==='roles')await user.validateSession();
  try{await load()}catch{}
  if(failed.length)ElMessage({type:ok?'warning':'error',duration:8000,showClose:true,message:'成功 '+ok+' 个，失败 '+failed.length+' 个：'+failed.join('；')});else ElMessage.success('批量操作完成，共 '+ok+' 个账号')}
async function openCreate(){try{const r:any=await request.get('/admin/employees',{params:{limit:1000,employmentScope:'all'}});employees.value=(r.data.items||[]).filter((e:any)=>!e.userId&&e.status===1);form.value={username:'',password:'',employeeId:''};visible.value=true}catch(e){report(e)}}
async function create(){if(!validPassword(form.value.password)){ElMessage.warning('密码需要6–72字节');return}saving.value=true;try{await request.post('/standalone/accounts',form.value);visible.value=false;form.value.password='';await load();ElMessage.success('子账号已创建')}catch(e){report(e)}finally{saving.value=false}}
const bindTarget=ref<any>(null),telegramId=ref(''),pendingAccounts=ref(new Set<string>());
function binding(row:any){bindTarget.value=row;telegramId.value=row.telegramUserId||'';bindVisible.value=true}
async function saveBinding(){if(!/^[1-9]\d{0,15}$/.test(telegramId.value.trim())){ElMessage.warning('请输入有效的数字 TG ID');return}saving.value=true;try{await request.post('/standalone/accounts/'+bindTarget.value.id+'/telegram',{telegramUserId:telegramId.value.trim()});bindVisible.value=false;await load();ElMessage.success('TG 绑定已保存')}catch(e){report(e)}finally{saving.value=false}}
async function deleteAccount(row:any){if(pendingAccounts.value.has(row.id))return;try{await box.confirm('将永久删除子账号 '+row.username+' 及其对应员工「'+(row.name||'-')+'」。考勤明细中的该员工、TG 绑定、角色和登录会话会一并删除，此操作不可恢复。确认删除？','删除子账号',{confirmButtonText:'确认删除',cancelButtonText:'取消',type:'warning',closeOnClickModal:false});pendingAccounts.value.add(row.id);await request.delete('/standalone/accounts/'+row.id);if(rows.value.length===1&&accountPage.value>1)accountPage.value--;await load();notifyEmployeesChanged('accounts');ElMessage.success('子账号及对应员工已删除')}catch(e){report(e)}finally{pendingAccounts.value.delete(row.id)}}
async function resetAttendance(row:any){if(pendingAccounts.value.has(row.id))return;let mode='';try{await box.confirm('请选择要对 '+row.name+' 执行的重置类型（仅影响当前业务日，原记录归档保留，【考勤明细】中的上班/下班时间不会改变）：<br><br><b>重置上下班</b>：上班、下班与活动次数全部重新开始。<br><b>重置活动次数</b>：只清零活动次数，上下班保持不变。','重置打卡',{dangerouslyUseHTMLString:true,distinguishCancelAndClose:true,showCancelButton:true,showClose:true,confirmButtonText:'重置上下班',cancelButtonText:'重置活动次数',type:'warning',closeOnClickModal:false});mode='all'}catch(a){if(a==='cancel')mode='activity';else return}pendingAccounts.value.add(row.id);try{await request.post('/standalone/accounts/'+row.id+'/reset-daily',{mode});ElMessage.success(mode==='all'?'已重置上下班与活动次数':'已重置活动次数');await load()}catch(e){report(e)}finally{pendingAccounts.value.delete(row.id)}}
async function toggle(row:any){if(pendingAccounts.value.has(row.id))return;pendingAccounts.value.add(row.id);try{await box.confirm('确认'+(row.status===1?'禁用':'启用')+' '+row.username+'？现有登录会话将失效。','账号状态');await request.put('/standalone/accounts/'+row.id,{status:row.status===1?0:1});await load()}catch(e){report(e)}finally{pendingAccounts.value.delete(row.id)}}
async function changePassword(){if(!validPassword(password.value.password)||password.value.password!==password.value.confirm){ElMessage.warning('请填写6–72字节的新密码，并确保两次输入一致');return}saving.value=true;try{await request.post('/auth/password',password.value);ElMessage.success('密码已修改，请重新登录');location.assign('/login')}catch(e){report(e)}finally{saving.value=false}}
const runStatus:Record<string,string>={completed:'完成',partial:'有异常',running:'执行中'};
const quotaVisible=ref(false);
const policy=ref<Record<string,any>>({auto:true,historyDays:30,logDays:180,tempHours:72,activityArchiveMonths:0,workArchiveMonths:0,monthlyStatsMonths:0,penaltyMonths:0,exportMonths:6}),quotaGiB=ref(0);
const runFields=[
  {key:'historyDays',label:'已处理消息 / 通知',hint:'已处理的收件箱消息与成员通知，至少 30 天',min:30,max:3650,unit:'天'},
  {key:'logDays',label:'维护日志',hint:'清理任务的执行日志，至少 90 天',min:90,max:3650,unit:'天'},
  {key:'tempHours',label:'导出临时副本',hint:'已有数据库备份的本地传输文件，至少 24 小时',min:24,max:8760,unit:'小时'}
];
const keepItems=[
  {key:'activityArchiveMonths',label:'活动归档',hint:'每日重置后归档的单条活动明细',def:12},
  {key:'workArchiveMonths',label:'上下班归档',hint:'每日重置后归档的单条上下班打卡明细',def:12},
  {key:'monthlyStatsMonths',label:'月度统计',hint:'按月汇总的活动与上班统计',def:24},
  {key:'penaltyMonths',label:'罚款记录',hint:'罚款详情中的历史罚款',def:12},
  {key:'exportMonths',label:'导出备份（XLSX）',hint:'每次重置前保存在数据库中的考勤表格，每次新增一份，占用空间最大',def:6}
];
function setKeep(it:{key:string;def:number},mode:string|number|boolean){policy.value[it.key]=mode==='forever'?0:it.def}
function bytes(n:number=0){if(!Number.isFinite(n)||n<=0)return '0 B';const units=['B','KiB','MiB','GiB','TiB'];const i=Math.min(4,Math.floor(Math.log(n)/Math.log(1024)));return (n/1024**i).toFixed(1)+' '+units[i]}
const memoryPage=ref(1),memoryLoading=ref(false);
function visibleAlerts(alerts:string[]=[]){return alerts.filter(a=>!a.startsWith('服务器磁盘使用率超过80%')&&!a.startsWith('未设置外部数据库配额'))}
async function changeMemoryPage(page:number){memoryPage.value=page;await loadMemory(false)}
async function loadMemory(refreshPolicy=true){memoryLoading.value=true;try{const r:any=await request.get('/standalone/memory',{params:{page:memoryPage.value,pageSize:10}});memory.value=r.data;memoryPage.value=r.data.page;if(refreshPolicy){policy.value={...r.data.policy};quotaGiB.value=r.data.policy.databaseQuotaBytes/1024**3}}catch(e){report(e)}finally{memoryLoading.value=false}}
async function savePolicy(){saving.value=true;try{const r:any=await request.put('/standalone/memory/policy',{...policy.value,databaseQuotaBytes:Math.round(quotaGiB.value*1024**3)});memory.value=r.data;memoryPage.value=r.data.page;ElMessage.success('清理策略已保存');return true}catch(e){report(e);return false}finally{saving.value=false}}
async function saveQuota(){if(await savePolicy())quotaVisible.value=false}
async function cleanMemory(){saving.value=true;try{const r:any=await request.post('/standalone/memory/clean');memory.value=r.data;memoryPage.value=r.data.page;if(r.data.history?.[0]?.status==='partial')ElMessage.warning('清理结束，部分项目异常，请查看执行日志');else ElMessage.success('清理结束，请查看实际空间变化')}catch(e){report(e);await loadMemory(false)}finally{saving.value=false}}
const resourceTimer=window.setInterval(()=>{if(mode.value==='accounts'&&!saving.value&&!document.hidden&&!loading.value)void load(true).catch(()=>{})},30000);
onUnmounted(()=>{window.clearInterval(resourceTimer);window.clearTimeout(searchTimer)});
onMounted(async()=>{try{if(mode.value==='roles')await loadRoles();else if(mode.value==='memory')await loadMemory();else if(mode.value==='accounts')await Promise.all([load(),loadLoginPolicy()]);else if(mode.value==='password')await loadProfile()}catch(e){report(e)}});
</script>
<style scoped>
.global-totp{display:flex;align-items:center;gap:10px;font-size:13px;color:#385b77}.totp-cell{display:flex;align-items:center;flex-wrap:wrap;gap:6px}.totp-cell small{width:100%;color:#71867d}

.list-pagination{margin-top:18px;overflow-x:auto}
.accounts-toolbar{padding-bottom:18px;border-bottom:1px solid #e8efec}
.accounts-actions{display:flex;align-items:center;gap:14px;flex-wrap:wrap;margin-left:auto}
.accounts-search{width:340px;max-width:100%}
.accounts-search :deep(.el-input__wrapper){border-radius:999px;padding:3px 16px;background:#fff;box-shadow:0 0 0 1px #dce7ef inset,0 2px 8px #1b3a3112;transition:box-shadow .2s}
.accounts-search :deep(.el-input__wrapper:hover){box-shadow:0 0 0 1px #b9cfd8 inset,0 2px 8px #1b3a3118}
.accounts-search :deep(.el-input__wrapper.is-focus){box-shadow:0 0 0 1.5px #279c8a inset,0 4px 12px #279c8a26}
.accounts-search .search-icon{color:#7a93a3;margin-right:2px}
.accounts-page .accounts-batch.el-button{height:34px;padding:0 20px;font-weight:600;transition:transform .15s,box-shadow .15s}
.accounts-page .accounts-batch.el-button:hover{transform:translateY(-1px);box-shadow:0 4px 12px #1b3a3118}
.accounts-page .accounts-add.el-button{height:34px;padding:0 20px;font-weight:600;box-shadow:0 4px 12px #279c8a40;transition:transform .15s,box-shadow .15s}
.accounts-page .accounts-add.el-button:hover{transform:translateY(-1px);box-shadow:0 6px 16px #279c8a55}
.row-ops{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.accounts-page :deep(.el-button.icon-btn){width:32px;height:32px;padding:0;margin:0}.accounts-page :deep(.el-button.icon-btn.el-button--warning){background:linear-gradient(120deg,#ffe9c7,#ffd9a0);color:#a8620f;border:0}.accounts-page :deep(.el-button.icon-btn.el-button--primary){border:0}
.accounts-add .plus{margin-right:4px;font-weight:700}
.accounts-batch .batch-count{display:inline-block;min-width:18px;height:18px;line-height:18px;margin-left:6px;padding:0 5px;border-radius:9px;background:#279c8a;color:#fff;font-size:12px;text-align:center}
.batch-types{margin-bottom:18px}.batch-body{min-height:90px}.batch-reset{display:flex;flex-direction:column;align-items:flex-start;gap:8px}.edit-tip{margin:8px 0 0;font-size:12px;line-height:1.5}
.batch-names{margin-top:14px;padding:10px 12px;max-height:84px;overflow:auto;border-radius:10px;background:#f4f8f6;color:#5b736c;font-size:12px;line-height:1.7;overflow-wrap:anywhere}
.accounts-toolbar .global-totp{box-sizing:border-box;height:34px;padding:0 8px 0 20px;border:0;border-radius:999px;color:#fff;font-size:14px;font-weight:600;background:linear-gradient(120deg,#279c8a,#6a79d7);box-shadow:0 4px 12px #279c8a40;--el-switch-off-color:#ffffff59}.accounts-toolbar .global-totp :deep(.el-switch.is-checked .el-switch__core){background:#1d5e54;border-color:transparent}
@media(max-width:700px){.accounts-actions{width:100%;margin-left:0}.accounts-search{width:100%}}
.accounts-page :deep(.el-button){border-radius:999px;background:linear-gradient(120deg,#edf7f4,#e9edff);border:1px solid #dce7ef;padding:8px 14px;color:#385b77}
.accounts-page :deep(.el-button--primary){background:linear-gradient(120deg,#279c8a,#6a79d7);color:white;border:0}
.accounts-page :deep(.el-button.is-link){margin:4px 6px 4px 0}
.accounts-page :deep(.el-switch.is-checked .el-switch__core){background:linear-gradient(120deg,#279c8a,#6a79d7);border-color:transparent}
.accounts-page :deep(.el-tag){border-radius:999px;background:linear-gradient(120deg,#edf7f4,#edf0ff)}
.accounts-page .panel{background:linear-gradient(135deg,#fff,#f7fbfa 60%,#f3f4ff);border-radius:24px}
.accounts-page :deep(.el-table){font-size:15px}.accounts-page :deep(.el-table th.el-table__cell){font-size:15px}.accounts-page :deep(.el-table th.el-table__cell.big-col .cell){font-size:15px;font-weight:600}.accounts-page :deep(.el-table .tg-col .el-button){font-size:15px}.accounts-page :deep(.el-table .big-col .cell){font-size:17px;font-weight:500}.accounts-page :deep(.el-table .op-col .el-button){font-size:14px}.accounts-page :deep(.el-table .el-tag){font-size:14px;height:28px}.accounts-page :deep(.el-table td.el-table__cell){padding:14px 0}.accounts-page .totp-cell small{font-size:13px}

.management-page{width:100%;margin:0}
.management-heading .muted{margin:0}
.eyebrow{display:block;color:#78948e;font-size:11px;letter-spacing:1px;margin-bottom:10px}
.management-page .panel{margin-top:0;width:100%;border-radius:14px;padding:24px 28px;box-shadow:0 2px 12px #1b3a3114;border-color:#e1eae6}
.management-page .panel+.panel{margin-top:20px}
.management-page .panel h3{font-size:16px;color:#1d3d36;padding-left:12px;border-left:3px solid #205c50;line-height:1.1}
.panel-toolbar{display:flex;justify-content:space-between;align-items:center;margin-bottom:20px;gap:12px;flex-wrap:wrap}
.panel-toolbar h3{margin:0!important}
.management-page :deep(.el-table){width:100%;border-radius:10px;overflow:hidden;--el-table-header-bg-color:#f2f7f4;--el-table-row-hover-bg-color:#f5faf7}
.management-page :deep(.el-table th.el-table__cell){color:#345c51;font-weight:600;height:46px}
.management-page :deep(.el-table td.el-table__cell){padding:12px 0}
.pw-page{--pw-ink:#17352f;--pw-text:#2b4a43;--pw-muted:#6a827c;--pw-line:#dfe8e4;--pw-accent:#205c50;--pw-focus:#279c8a;display:grid;grid-template-columns:minmax(0,460px) minmax(0,560px);gap:28px;align-items:start;justify-content:start}
.pw-card{background:#fff;border:1px solid var(--pw-line);border-radius:18px;padding:32px 36px 36px;box-shadow:0 1px 2px #102b2d0a,0 18px 40px -18px #102b2d33}
.pw-head{display:flex;gap:16px;align-items:flex-start;padding-bottom:24px;margin-bottom:26px;border-bottom:1px solid #edf2ef}
.pw-badge{display:grid;place-items:center;flex:0 0 46px;height:46px;border-radius:14px;background:#e6f1ed;color:var(--pw-accent)}
.pw-head h2{margin:2px 0 6px;font-size:20px;line-height:1.3;font-weight:650;color:var(--pw-ink)}
.pw-head p{margin:0;font-size:13px;line-height:1.7;color:var(--pw-muted)}
.pw-head p b{color:var(--pw-text);font-weight:600}
.pw-form :deep(.el-form-item){margin-bottom:24px}
.pw-form :deep(.el-form-item__label){padding-bottom:8px;line-height:1.2;font-size:13px;font-weight:600;color:var(--pw-text)}
.pw-form :deep(.el-input__wrapper){height:46px;padding:0 14px;border-radius:10px;background:#f7faf9;box-shadow:0 0 0 1px #d6e2dd inset;transition:box-shadow .18s,background-color .18s}
.pw-form :deep(.el-input__wrapper:hover){box-shadow:0 0 0 1px #b3c9c1 inset}
.pw-form :deep(.el-input__wrapper.is-focus){background:#fff;box-shadow:0 0 0 1.5px var(--pw-focus) inset,0 0 0 4px #279c8a22}
.pw-form :deep(.el-input__inner){font-size:15px;color:var(--pw-ink)}
.pw-form :deep(.el-input__inner::placeholder){color:#9db0aa}
.pw-meter{flex:0 0 100%;display:grid;grid-template-columns:1fr auto;grid-template-areas:"bars bars" "text bytes";gap:8px 12px;margin-top:12px;font-size:12px;line-height:1.4}
.pw-bars{grid-area:bars;display:grid;grid-template-columns:repeat(4,1fr);gap:6px}
.pw-bars i{height:4px;border-radius:2px;background:#e3eae7;transition:background-color .2s}
.lv-1 .pw-bars i:nth-child(-n+1){background:#d4685c}
.lv-2 .pw-bars i:nth-child(-n+2){background:#e0a43a}
.lv-3 .pw-bars i:nth-child(-n+3){background:#5aa788}
.lv-4 .pw-bars i:nth-child(-n+4){background:#1f7a66}
.pw-meter-text{grid-area:text;color:var(--pw-muted)}
.pw-bytes{grid-area:bytes;color:#8aa098;font-variant-numeric:tabular-nums}
.pw-meter-text.bad,.pw-bytes.bad{color:#c0483c}
.pw-match{flex:0 0 100%;margin:10px 0 0;font-size:12px;line-height:1.4}
.pw-match.ok{color:#1f7a66}
.pw-match.bad{color:#c0483c}
.pw-form .pw-submit.el-button{display:block;width:100%;height:48px;margin-top:6px;border-radius:12px;font-size:15px;font-weight:600;letter-spacing:.5px;box-shadow:0 8px 18px -8px #205c5099;transition:transform .15s,box-shadow .15s,background-color .15s}
.pw-form .pw-submit.el-button:hover{transform:translateY(-1px);box-shadow:0 12px 22px -10px #205c50b3}
.pw-form .pw-submit.el-button:active{transform:none}
.pw-form .pw-submit.el-button:focus-visible{outline:3px solid #279c8a55;outline-offset:2px}
.pw-side{background:#fff;border:1px solid var(--pw-line);border-radius:18px;padding:8px 24px;box-shadow:0 1px 2px #102b2d0a,0 18px 40px -18px #102b2d33;min-width:0;position:sticky;top:16px}
.pw-side-sec{padding:20px 0}
.pw-side-sec+.pw-side-sec{border-top:1px solid #edf2ef}
.pw-side h3{display:flex;align-items:center;gap:10px;margin:0 0 14px;font-size:14px;font-weight:650;color:var(--pw-ink)}
.pw-side-ico{display:grid;place-items:center;flex:0 0 28px;height:28px;border-radius:9px;background:#e6f1ed;color:var(--pw-accent)}
.pw-rules{list-style:none;margin:0;padding:0;display:grid;gap:12px}
.pw-rules li{display:flex;gap:10px;align-items:flex-start;font-size:13px;line-height:1.5;color:var(--pw-muted);transition:color .2s}
.pw-rules li svg{flex:0 0 18px;margin-top:1px;fill:none;stroke:#c3d1cc;stroke-width:1.5;stroke-linecap:round;stroke-linejoin:round;transition:fill .2s,stroke .2s}
.pw-rules li small{display:block;margin-top:2px;font-size:12px;color:#93a69f}
.pw-rules li.ok{color:var(--pw-text)}
.pw-rules li.ok svg{fill:#1f7a66;stroke:#fff}
.pw-rules li.ok svg circle{stroke:#1f7a66}
.pw-note{margin:0 0 10px;font-size:13px;line-height:1.8;color:var(--pw-muted);overflow-wrap:anywhere}.pw-note:last-child{margin-bottom:0}
@media(prefers-reduced-motion:reduce){.pw-page *{transition:none!important}}
.resource-alert{margin-bottom:12px}
.resource-metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:18px;margin:4px 0 26px}
.resource-metrics article{border:1px solid #dbe8e1;border-radius:14px;padding:22px 24px;background:linear-gradient(160deg,#fff,#f3f9f5);transition:box-shadow .2s,transform .2s}
.resource-metrics article:hover{box-shadow:0 6px 18px #1b3a3120;transform:translateY(-2px)}
.resource-metrics article>span{font-size:13px;color:#72827e}
.resource-metrics strong{display:block;font-size:30px;color:#205c50;margin:14px 0}
.resource-metrics small{display:block;color:#71808e;line-height:1.8;margin-top:12px}
.resource-panel .muted{max-width:none}
.resource-panel>h3{margin-top:26px}
.policy-head{display:flex;justify-content:space-between;align-items:flex-start;gap:18px;margin-top:26px}
.policy-head h3{margin:0 0 6px}
.policy-head .muted{margin:0}
.policy-board{display:flex;flex-direction:column;gap:18px;margin:16px 0 20px}
.policy-top{display:flex;flex-wrap:wrap;gap:18px;align-items:stretch}
.pc-auto{flex:1 1 320px;min-width:0}
.pc-run{flex:2 1 520px;min-width:0}
.policy-card{border:1px solid #dbe8e1;border-radius:14px;padding:18px 20px 20px;background:linear-gradient(160deg,#fff,#f6faf7);box-shadow:0 1px 3px #1b3a3110}
.pc-head{display:flex;align-items:center;gap:12px;margin-bottom:16px;padding-bottom:14px;border-bottom:1px dashed #dbe8e1}
.pc-ico{display:grid;place-items:center;width:38px;height:38px;border-radius:10px;background:#e6f2ec;font-size:18px;flex:none}
.pc-title{flex:1;min-width:0}
.pc-title b{display:block;font-size:15px;color:#205c50}
.pc-title small{display:block;margin-top:3px;color:#71808e;line-height:1.5}
.pc-badge{flex:none;font-size:12px;padding:2px 10px;border-radius:999px;font-weight:600}
.pc-badge.safe{background:#e3f4ea;color:#2a7a57}
.pc-badge.warn{background:#fff1dc;color:#b7791f}
.pc-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:16px 20px}
.pc-field label{display:block;font-size:14px;font-weight:600;color:#2f4a43;margin-bottom:2px}
.pc-field small{display:block;color:#7a8a86;line-height:1.5;margin-bottom:8px;min-height:18px}
.pc-run .pc-field label{display:flex;align-items:center;margin-bottom:8px}
.hint-btn{display:inline-grid;place-items:center;width:16px;height:16px;margin-left:6px;padding:0;border:1px solid #9bb5ab;border-radius:50%;background:#fff;color:#5d8577;font-size:11px;font-weight:600;line-height:1;cursor:pointer;transition:all .15s}
.hint-btn:hover,.hint-btn:focus-visible{background:#205c50;border-color:#205c50;color:#fff;outline:none}
.unit-input{display:flex;align-items:center;gap:8px}
.unit-input :deep(.el-input-number){flex:1;min-width:0}
.unit-input em{font-style:normal;color:#72827e;font-size:13px;flex:none}
.unit-input.sm{margin-top:8px}
.keep-card{border-color:#f0dfc2;background:linear-gradient(160deg,#fff,#fffaf1)}
.keep-card .pc-head{border-bottom-color:#f0dfc2}
.keep-card .pc-ico{background:#fff1dc}
.keep-list{display:flex;flex-wrap:wrap;gap:12px 20px}
.keep-row{flex:1 1 340px;min-width:0;display:flex;justify-content:space-between;align-items:center;gap:14px;flex-wrap:wrap;padding:12px 14px;border:1px solid #ece3d2;border-radius:10px;background:#fff;transition:border-color .2s,background .2s}
.keep-row.on{border-color:#e6b980;background:#fffaf0}
.keep-info{flex:1;min-width:160px}
.keep-info b{display:block;color:#2f4a43;font-size:14px}
.keep-info small{display:block;margin-top:3px;color:#7a8a86;line-height:1.5}
.keep-ctl{display:flex;flex-direction:column;align-items:flex-end}
.keep-note{margin:14px 2px 0;padding:10px 14px;border-radius:8px;background:#fff1dc;color:#8a5a14;font-size:13px;line-height:1.7}
@media (max-width:640px){.policy-head{flex-direction:column}.keep-ctl{align-items:flex-start}}
.resource-detail{padding:12px 30px}
.resource-error{color:#c45656}
.roles-head{display:flex;justify-content:space-between;align-items:center;gap:16px;margin-bottom:18px;flex-wrap:wrap}
.roles-head h3{margin:0!important}
.roles-actions{display:flex;align-items:center;gap:12px;flex-wrap:wrap}
.roles-search{width:300px;max-width:100%}
.roles-table :deep(th.el-table__cell){background:#f5f8f7;color:#4b635c;font-weight:600}
.roles-table :deep(.el-table__row:hover>td){background:#f4faf8!important}
.role-cell{display:flex;align-items:center;gap:12px;padding:4px 0}
.role-avatar{flex:none;width:36px;height:36px;border-radius:10px;display:grid;place-items:center;font-weight:700;font-size:15px;color:#fff;background:#6366f1}
.role-avatar.system{background:linear-gradient(135deg,#205c50,#3a9a84)}.role-avatar.default{background:linear-gradient(135deg,#64748b,#94a3b8)}.role-avatar.custom{background:linear-gradient(135deg,#6366f1,#8b5cf6)}
.role-main{min-width:0;display:flex;flex-direction:column;gap:3px}
.role-name{display:flex;align-items:center;gap:8px}.role-name b{font-size:14px;color:#1d3d36}
.role-desc{color:#8a9a95;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:260px}
.role-members{display:flex;align-items:baseline;gap:8px;flex-wrap:wrap}
.role-members b{font-size:16px;color:#205c50}.role-members b.zero{color:#c0c4cc}
.role-member-names{font-size:12px;color:#7a8c86}
.role-member-count{min-width:34px;height:28px;padding:0 10px;border:0;border-radius:14px;font-size:14px;font-weight:700;color:#fff;cursor:pointer;background:linear-gradient(120deg,#205c50,#1f7a78 50%,#229ed9);box-shadow:0 6px 14px -6px rgba(34,158,217,.7);transition:transform .2s,box-shadow .25s}
.role-member-count:hover{transform:translateY(-1px) scale(1.06);box-shadow:0 10px 20px -6px rgba(34,158,217,.9)}
.role-member-tip{max-width:320px;line-height:1.7}.role-member-tip small{opacity:.75}
.member-summary{margin-bottom:14px;color:#5b6f75;font-size:13px}.member-summary b{color:#205c50;font-size:16px}
.member-list{display:grid;gap:12px;min-height:120px}
.member-card{display:flex;gap:14px;align-items:center;padding:14px;border-radius:14px;background:rgba(255,255,255,.75);border:1px solid rgba(32,92,80,.10);transition:transform .25s,box-shadow .25s,border-color .25s}
.member-card:hover{transform:translateY(-2px);border-color:rgba(34,158,217,.4);box-shadow:0 14px 28px -16px rgba(34,158,217,.7)}
.member-card.disabled{opacity:.7}
.member-avatar{position:relative;flex:0 0 44px;width:44px;height:44px;border-radius:50%;display:grid;place-items:center;font-size:18px;font-weight:700;color:#fff;background:linear-gradient(135deg,#205c50,#229ed9 60%,#8b5cf6)}
.member-online{position:absolute;right:0;bottom:0;width:11px;height:11px;border-radius:50%;background:#2ecc71;border:2px solid #fff;box-shadow:0 0 8px #2ecc71}
.member-main{min-width:0;flex:1}.member-title{display:flex;gap:8px;align-items:center}.member-title b{font-size:15px;color:#233146}
.member-meta{display:flex;gap:14px;flex-wrap:wrap;margin-top:4px;font-size:12px;color:#7a8c86}

.role-perms{display:flex;flex-wrap:wrap;gap:6px;align-items:center}
.role-chip{display:inline-flex;align-items:center;gap:5px;padding:2px 9px;border-radius:999px;font-size:12px;background:#e9f4f0;color:#205c50;border:1px solid #d3e8e0}
.role-chip em{font-style:normal;font-weight:700;padding:0 5px;border-radius:999px;background:#205c50;color:#fff;font-size:11px}
.role-chip.more{background:#f1f3f5;color:#6b7c76;border-color:#e2e6e8}
.role-total{color:#a0aeaa;margin-left:2px}
.role-none{font-size:12px;color:#e6a23c}
.role-time{font-size:12px;color:#7a8c86}
.permission-summary{display:flex;justify-content:space-between;align-items:center;margin-top:16px;font-size:13px;color:#4b635c}
.permission-summary b{color:#205c50;font-size:15px}
.permission-row strong small{display:block;margin-top:3px;font-weight:400;font-size:11px;color:#9aaba5}
.permission-matrix{margin-top:10px;border:1px solid #e5ece9;border-radius:12px;overflow:hidden;max-height:52vh;overflow-y:auto}
.permission-row{display:grid;grid-template-columns:140px 1fr;gap:20px;padding:15px 20px;border-bottom:1px solid #edf1ef;align-items:center}
.permission-row:last-child{border:0}
.permission-row:nth-child(odd){background:#f7faf8}
.permission-row strong{font-size:14px;color:#345c51}
.profile-avatar-row{display:flex;align-items:center;gap:22px;margin-bottom:26px}.profile-avatar{flex:none;border:3px solid #e6f1ed;box-shadow:0 6px 16px -8px #205c5099;background:#f0f5f3;color:#6a827c}.profile-avatar-actions .pw-note{margin:10px 0 0}
.profile-list{margin:0;display:grid;gap:0;border-top:1px solid #edf2ef}.profile-list>div{display:flex;gap:16px;padding:14px 0;border-bottom:1px solid #edf2ef;font-size:14px}.profile-list dt{flex:0 0 76px;color:var(--pw-muted)}.profile-list dd{margin:0;color:var(--pw-ink);font-weight:550;overflow-wrap:anywhere}
@media(max-width:1000px){.pw-page{grid-template-columns:minmax(0,560px)}}
@media(max-width:700px){.permission-row{grid-template-columns:1fr;gap:6px}.management-heading{display:block;padding:18px}.pw-card{padding:22px 20px 26px;border-radius:14px}.pw-head{padding-bottom:18px;margin-bottom:20px}.pw-page{gap:20px}.pw-side{padding:4px 18px;border-radius:14px}.pw-side-sec{padding:16px 0}.management-page .panel{padding:18px}}
</style>
<style>
/* 删除按钮：粉红渐变（对话框是挂到 body 的，所以放在非 scoped 样式里） */
.el-button.del-btn,.management-page.accounts-page .el-button.del-btn{border:0;color:#fff;background:linear-gradient(120deg,#ff9aa8,#f0527a);box-shadow:0 4px 10px #f0527a40;font-weight:600}
.el-button.del-btn:hover,.el-button.del-btn:focus-visible,.management-page.accounts-page .el-button.del-btn:hover{color:#fff;background:linear-gradient(120deg,#ff8496,#e63f68);box-shadow:0 6px 14px #f0527a59}
.el-button.del-btn.is-loading,.el-button.del-btn.is-disabled{opacity:.7}

/* 弹窗圆润样式：对话框与确认框都挂到 body，所以放在非 scoped 样式里 */
.el-dialog.pill-dialog,.el-message-box.pill-dialog{border-radius:22px;padding:22px 26px 20px;overflow:hidden;box-shadow:0 24px 60px -20px #102b2d55}
.pill-dialog .el-dialog__header,.pill-dialog .el-message-box__header{padding-bottom:12px}
.pill-dialog .el-dialog__title,.pill-dialog .el-message-box__title{font-weight:650;color:#17352f}
.pill-dialog .el-dialog__footer,.pill-dialog .el-message-box__btns{padding-top:14px}
.pill-dialog .el-input__wrapper{border-radius:999px;padding:3px 16px;box-shadow:0 0 0 1px #dce7ef inset}
.pill-dialog .el-input__wrapper.is-focus{box-shadow:0 0 0 1.5px #279c8a inset,0 4px 12px #279c8a26}
.pill-dialog .el-select__wrapper{border-radius:18px;box-shadow:0 0 0 1px #dce7ef inset}
.pill-dialog .el-alert{border-radius:14px}
.pill-dialog .el-button:not(.del-btn){height:34px;padding:0 20px;border-radius:999px;border:1px solid #dce7ef;background:linear-gradient(120deg,#edf7f4,#e9edff);color:#385b77;font-weight:600;box-shadow:none;transition:transform .15s,box-shadow .15s}
.pill-dialog .el-button:not(.del-btn):hover{border-color:#b9cfd8;color:#205c50;transform:translateY(-1px);box-shadow:0 4px 12px #1b3a3118}
.pill-dialog .el-button--primary:not(.del-btn){border:0;color:#fff;background:linear-gradient(120deg,#279c8a,#6a79d7);box-shadow:0 4px 12px #279c8a40}
.pill-dialog .el-button--primary:not(.del-btn):hover{color:#fff;box-shadow:0 6px 16px #279c8a55}
.pill-dialog .el-button.del-btn{height:34px;padding:0 20px;border-radius:999px}
</style>
