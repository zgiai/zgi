'use client';

import { acceptInvite, getOrgInfoByToken } from '@/services/organization';
import { message } from 'antd';
import { motion } from 'framer';
import { useState, useEffect } from 'react';

export default function InvitePage({ token }: { token: string }) {
    const [isHovered, setIsHovered] = useState(false);
    const [isLoading, setIsLoading] = useState(false);
    const [orgName, setOrgName] = useState('');
    
    const acceptInvitation = async () => {
        setIsLoading(true);
        try {
            const res = await acceptInvite({ invite_token: token });
            if (res.status_code === 200) {
                message.success('Invitation accepted successfully');
                location.href = '/';
            } else if (res.status_code === 400) {
                message.error("You have already accepted the invitation");
                location.href = '/';
            } else {
                message.error(res.status_message || 'Error accepting invitation');
            }
        } catch (error) {
            console.error(error);
        }finally {
            setIsLoading(false);
        }
    };

    const getOrgName = async () => {
        try {
            const res = await getOrgInfoByToken({ invite_token: token });
            if (res?.status_code === 200) {
                setOrgName(res?.data?.name);
            } else {
                console.error('Error accepting invitation');
            }
        } catch (error) {
            console.error(error);
        }
    };

    useEffect(() => {
        getOrgName();
    }, []);

    return (
        <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6 }}
            className="min-h-screen bg-gradient-to-b from-gray-100 to-gray-50 dark:from-gray-900 dark:to-gray-800"
        >
            <main className="pt-24 pb-16 px-4 sm:px-6 lg:px-8">
                <div className="max-w-3xl mx-auto">
                    <motion.div
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        transition={{ delay: 0.2 }}
                        className="bg-white dark:bg-gray-800 rounded-2xl shadow-lg p-8"
                    >
                        <div className="text-center">
                            <motion.h1
                                initial={{ opacity: 0, y: 20 }}
                                animate={{ opacity: 1, y: 0 }}
                                transition={{ delay: 0.3 }}
                                className="text-3xl font-bold text-gray-900 dark:text-white mb-4"
                            >
                                You're Invited!
                            </motion.h1>
                            <motion.p
                                initial={{ opacity: 0 }}
                                animate={{ opacity: 1 }}
                                transition={{ delay: 0.4 }}
                                className="text-gray-600 dark:text-gray-300 mb-8"
                            >
                                You've been invited to join our organization.
                            </motion.p>
                            <motion.p
                                initial={{ opacity: 0 }}
                                animate={{ opacity: 1 }}
                                transition={{ delay: 0.4 }}
                                className="text-blue-600 mb-8 font-bold text-2xl"
                            >
                                {orgName}
                            </motion.p>
                            <motion.p
                                initial={{ opacity: 0 }}
                                animate={{ opacity: 1 }}
                                transition={{ delay: 0.4 }}
                                className="text-gray-600 dark:text-gray-300 mb-8"
                            >
                                Click below to accept the invitation.
                            </motion.p>
                            <motion.button
                                whileHover={{ scale: 1.02 }}
                                whileTap={{ scale: 0.98 }}
                                onHoverStart={() => setIsHovered(true)}
                                onHoverEnd={() => setIsHovered(false)}
                                className="w-full sm:w-auto inline-flex items-center justify-center px-8 py-3 border border-transparent text-base font-medium rounded-lg text-white bg-blue-600 hover:bg-blue-700 transition-colors duration-200 mb-4"
                                onClick={acceptInvitation}
                                disabled={isLoading}
                                style={{
                                    backgroundColor: isHovered ? '#3b82f6' : '#2563eb',
                                    borderColor: isHovered ? '#3b82f6' : '#2563eb',
                                }}
                            >
                                {isLoading ? 'Accepting...' : 'Accept Invitation'}
                            </motion.button>
                        </div>
                    </motion.div>
                </div>
            </main>
        </motion.div>
    );
}