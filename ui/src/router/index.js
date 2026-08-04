import { createRouter, createWebHashHistory } from 'vue-router'
import { globalState } from '../composables/useGlobalState'
import { configStore } from '../stores/app.js'

import Dashboard from '../views/Dashboard.vue'
import ChannelList from '../views/ChannelList.vue'
import DeviceList from '../views/DeviceList.vue'
import PointList from '../views/PointList.vue'
import Northbound from '../views/Northbound.vue'
import VirtualShadowDevices from '../views/VirtualShadowDevices.vue'
import EdgeCompute from '../views/EdgeCompute.vue'
import SystemSettings from '../views/SystemSettings.vue'
import LogViewer from '../views/LogViewer.vue'
import Login from '../views/Login.vue'
import NodeSync from '../views/NodeSync.vue'
import Install from '../views/Install.vue'

const routes = [
    {
        path: '/install',
        component: Install,
        meta: { title: '系统安装配置', level: 0 }
    },
    {
        path: '/login',
        component: Login,
        meta: { title: '登录', level: 0 }
    },
    {
        path: '/',
        component: Dashboard,
        meta: { title: '首页监控', level: 1 }
    },
    {
        path: '/logs',
        component: LogViewer,
        meta: { title: '系统日志', level: 1 }
    },
    {
        path: '/system',
        component: SystemSettings,
        meta: { title: '系统设置', level: 1 }
    },
    {
        path: '/channels',
        component: ChannelList,
        meta: { title: '采集通道', level: 1 }
    },
    {
        path: '/edge-compute',
        component: EdgeCompute,
        meta: { title: '边缘计算', level: 1 }
    },
    {
        path: '/channels/:channelId/devices',
        component: DeviceList,
        meta: { title: '设备列表', level: 2 }
    },
    {
        // (.*) allows OPC UA endpoint-style device IDs containing / and :
        path: '/channels/:channelId/devices/:deviceId(.*)/points',
        component: PointList,
        meta: { title: '点位数据', level: 3 }
    },
    {
        path: '/virtual-shadows',
        component: VirtualShadowDevices,
        meta: { title: '虚拟设备', level: 1 }
    },
    {
        path: '/northbound',
        component: Northbound,
        meta: { title: '北向上报', level: 1 }
    },
    {
        path: '/node-sync',
        component: NodeSync,
        meta: { title: '节点同步', level: 1 }
    }
]

const router = createRouter({
    history: createWebHashHistory(),
    routes
})

router.beforeEach(async (to, from, next) => {
    globalState.navTitle = '';

    const publicPages = ['/login', '/install'];
    const authRequired = !publicPages.includes(to.path);

    const config = configStore()

    const installed = await config.checkInstallStatus()

    if (!installed && to.path !== '/install') {
        return next('/install');
    }

    if (installed && to.path === '/install') {
        return next('/login');
    }

    let hasValidToken = false
    const stored = localStorage.getItem('loginInfo')
    if (stored) {
        try {
            const parsed = JSON.parse(stored)
            if (parsed && parsed.token) {
                hasValidToken = true
            } else {
                localStorage.removeItem('loginInfo')
            }
        } catch (e) {
            localStorage.removeItem('loginInfo')
        }
    }

    if (authRequired && !hasValidToken) {
        return next('/login');
    }

    next();
})

export default router
