package tags

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetConfigMapTags 获取 cm 中的标签
func GetConfigMapTags(client kubernetes.Interface, configMapNamespace, configMapName string) (map[string]string, error) {
	ctx := context.Background()
	cm, err := client.CoreV1().ConfigMaps(configMapNamespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			createErr := CreateEmptyConfigMap(client, configMapNamespace, configMapName)
			if createErr != nil {
				return nil, createErr
			}
			return nil, createErr
		}
		return nil, err
	}

	return cm.Data, err
}

// CreateEmptyConfigMap 创建空的 cm
func CreateEmptyConfigMap(client kubernetes.Interface, configMapNamespace, configMapName string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
		},
		Data: nil,
	}

	ticker := time.NewTicker(time.Second * 5)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			_, err := client.CoreV1().ConfigMaps(configMapNamespace).Create(ctx, cm, metav1.CreateOptions{})
			if err != nil && !errors.IsAlreadyExists(err) {
				glog.Warningf("CreateEmptyConfigMap failed, err: %v", err)
				continue
			}
			return nil
		case <-ctx.Done():
			return fmt.Errorf("CreateEmptyConfigMap failed before deadline exceeded")
		}
	}
}

// UpdateConfigMap 更新 cm
func UpdateConfigMap(client kubernetes.Interface, clusterTags map[string]string, configMapNamespace, configMapName string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
		},
		Data: clusterTags,
	}

	ticker := time.NewTicker(time.Second * 5)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			_, err := client.CoreV1().ConfigMaps(configMapNamespace).Update(ctx, cm, metav1.UpdateOptions{})
			if err != nil {
				glog.Warningf("UpdateConfigMap failed, err: %v", err)
				continue
			}
			return err
		case <-ctx.Done():
			return fmt.Errorf("UpdateConfigMap failed before deadline exceeded")
		}
	}
}
