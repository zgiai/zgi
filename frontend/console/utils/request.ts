/*eslint-disable*/
import axios, { AxiosResponse, InternalAxiosRequestConfig } from 'axios'

declare module 'axios' {
    export interface InternalAxiosRequestConfig {
        metadata?: {
            startTime: number;
        };
    }
}

// axios.defaults.withCredentials = true
// axios.defaults.timeout = 50000
// axios.defaults.headers.common['Content-Type'] = 'application/json'

// axios.interceptors.request.use(
//     (config: InternalAxiosRequestConfig) => {
//         config.metadata = {
//             startTime: new Date().getTime()
//         }

//         const token = localStorage.getItem("token")

//         if (token) {
//             config.headers.Authorization = `Bearer ${token}`;
//         }
//         if (!(config?.headers && config?.headers?.Authorization)) {
//             return config
//         } else {
//             config.withCredentials = false
//         }
//         return config
//     },
//     (err: any) => {
//         return Promise.reject(err)
//     }
// )

// axios.interceptors.response.use(
//     (response: AxiosResponse) => {
//         return response
//     },
//     (err: any) => {
//         if (!err.response) {
//             return
//         }
//         const res = err.response
//         if ([500, 502, 503].includes(res.status)) {
//             console.error('500 server error, please try again later!')
//         } else if (res.status === 401) {
//             console.error('Login authorization is required')
//         } else if (res.status === 403) {
//             console.error('Sorry! You do not have permission to operate this function')
//         } else if ([400, 404].includes(res.status)) {
//             console.error('400/404 interface request failed, please try again! If you have any questions, please contact the administrator.')
//         }
//     }
// )

// /**
//  * get
//  * @param url
//  * @param params
//  * @returns {Promise}
//  */
// const get = (url: string, params: Record<string, any> = {}): Promise<any> => {
//     params.withCredentials = true
//     return new Promise((resolve, reject) => {
//         axios
//             .get(url, { params })
//             .then(response => {
//                 resolve(response?.data || { data: [] })
//             })
//             .catch(error => {
//                 reject(error)
//             })
//     })
// }

// /**
//  * post
//  * @param url
//  * @param data
//  * @param params
//  * @returns {Promise}
//  */
// const post = (url: string, data: Record<string, any> = {}, params: Record<string, any> = {}): Promise<any> => {
//     return new Promise((resolve, reject) => {
//         axios.post(url, data, { params }).then(
//             response => {
//                 resolve(response.data || {})
//             },
//             error => {
//                 reject(error)
//             }
//         )
//     })
// }

// /**
//  * put
//  * @param url
//  * @param params
//  * @param query
//  * @returns {Promise}
//  */
// const put = (url: string, params: Record<string, any> = {}, query: Record<string, any> = {}): Promise<any> => {
//     return new Promise((resolve, reject) => {
//         axios.put(url, params, { params: query }).then(
//             response => {
//                 resolve(response?.data)
//             },
//             err => {
//                 reject(err)
//             }
//         )
//     })
// }

// /**
//  * del
//  * @param url
//  * @param params
//  * @returns {Promise}
//  */
// const del = (url: string, params: Record<string, any> = {}): Promise<any> => {
//     return new Promise((resolve, reject) => {
//         axios.delete(url, { params }).then(
//             response => {
//                 resolve(response?.data)
//             },
//             err => {
//                 reject(err)
//             }
//         )
//     })
// }

class AxiosInstance {
    private instance;

    constructor() {
        this.instance = axios.create({
            withCredentials: false,
            timeout: 50000,
            headers: {
                'Content-Type': 'application/json'
            }
        });

        this.instance.interceptors.request.use(
            (config: InternalAxiosRequestConfig) => {
                config.metadata = {
                    startTime: new Date().getTime()
                };

                const token = localStorage.getItem("token");

                if (token) {
                    config.headers.Authorization = `Bearer ${token}`;
                }
                if (!(config?.headers && config?.headers?.Authorization)) {
                    return config;
                } else {
                    config.withCredentials = false;
                }
                return config;
            },
            (err: any) => {
                return Promise.reject(err);
            }
        );

        this.instance.interceptors.response.use(
            (response: AxiosResponse) => {
                return response;
            },
            (err: any) => {
                if (!err.response) {
                    return;
                }
                const res = err.response;
                if ([500, 502, 503].includes(res.status)) {
                    console.error('500 server error, please try again later!');
                } else if (res.status === 401) {
                    console.error('Login authorization is required');
                } else if (res.status === 403) {
                    console.error('Sorry! You do not have permission to operate this function');
                } else if ([400, 404].includes(res.status)) {
                    console.error('400/404 interface request failed, please try again! If you have any questions, please contact the administrator.');
                }
            }
        );
    }

    get(url: string, params: Record<string, any> = {}): Promise<any> {
        // params.withCredentials = true;
        return new Promise((resolve, reject) => {
            this.instance
                .get(url, { params })
                .then(response => {
                    resolve(response?.data || { data: [] });
                })
                .catch(error => {
                    reject(error);
                });
        });
    }

    post(url: string, data: Record<string, any> = {}, params: Record<string, any> = {}): Promise<any> {
        return new Promise((resolve, reject) => {
            this.instance.post(url, data, { params }).then(
                response => {
                    resolve(response.data || {});
                },
                error => {
                    reject(error);
                }
            );
        });
    }

    put(url: string, params: Record<string, any> = {}, query: Record<string, any> = {}): Promise<any> {
        return new Promise((resolve, reject) => {
            this.instance.put(url, params, { params: query }).then(
                response => {
                    resolve(response?.data);
                },
                err => {
                    reject(err);
                }
            );
        });
    }

    del(url: string, params: Record<string, any> = {}): Promise<any> {
        return new Promise((resolve, reject) => {
            this.instance.delete(url, { params }).then(
                response => {
                    resolve(response?.data);
                },
                err => {
                    reject(err);
                }
            );
        });
    }
}

export default new AxiosInstance();