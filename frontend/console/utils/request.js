/*eslint-disable*/
import axios from 'axios'

axios.defaults.withCredentials = true
axios.defaults.timeout = 50000
axios.defaults.headers.common['Content-Type'] = 'application/json'

// 
axios.interceptors.request.use(
    config => {
        config.metadata = {
            startTime: new Date().getTime()
        }

        let token = localStorage.getItem("token")

        if (token) {
            config.headers.Authorization = `Bearer ${token}`
        }
        if (!(config?.headers && config?.headers?.Authorization)) {
            return config
        } else {
            config.withCredentials = false
        }
        return config
    },
    err => {
        return Promise.reject(err)
    }
)

axios.interceptors.response.use(
    response => {
        return response
    },
    err => {
        if (!err.response) {
            return
        }
        const res = err.response
        if ([500, 502, 503].indexOf(res.status) > -1) {
            console.error('500 server error, please try again later!')
        } else if (res.status === 401) {
            console.error('Login authorization is required')
        } else if (res.status === 403) {
            console.error('Sorry! You do not have permission to operate this function')
        } else if ([400, 404].indexOf(res.status) > -1) {
            console.error('400/404 interface request failed, please try again! If you have any questions, please contact the administrator.')
        }
    }
)

/**
 * get
 * @param url
 * @param data
 * @returns {Promise}
 */
const get = (url, params = {}) => {
    params.withCredentials = true
    return new Promise((resolve, reject) => {
        axios
            .get(url, { params })
            .then(response => {
                resolve(response?.data || { data: [] })
            })
            .catch(error => {
                reject(error)
            })
    })
}

/**
 * post
 * @param url
 * @param data
 * @returns {Promise}
 */
const post = (url, data = {}, params = {}) => {
    return new Promise((resolve, reject) => {
        axios.post(url, data, { params }).then(
            response => {
                resolve(response.data || {})
            },
            error => {
                reject(error)
            }
        )
    })
}

/**
 * put
 * @param url
 * @param data
 * @returns {Promise}
 */

const put = (url, params = {}, query = {}) => {
    return new Promise((resolve, reject) => {
        axios.put(url, params, { params: query }).then(
            response => {
                resolve(response?.data)
            },
            err => {
                reject(err)
            }
        )
    })
}

/**
 * del
 * @param url
 * @param data
 * @returns {Promise}
 */

const del = (url, params = {}) => {
    return new Promise((resolve, reject) => {
        axios.delete(url, { params }).then(
            response => {
                resolve(response?.data)
            },
            err => {
                reject(err)
            }
        )
    })
}

export default {
    get,
    post,
    put,
    del
}