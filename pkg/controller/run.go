package controller

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/sethvargo/go-password/password"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

type Secret struct {
	SlackBotToken string `yaml:"slack_bot_token"`
}

func (ctrl *Controller) Run(ctx context.Context, param Param) error {
	sess := session.Must(session.NewSession())
	logE := logrus.WithFields(logrus.Fields{
		"user_name": param.UserName,
	})

	// get a slack user id
	logE.Info("start getting a Slack User")
	user, err := ctrl.GetSlackUser(ctx, param.UserName)
	if err != nil {
		if ctrl.Config.Slack.ChannelIDForSystemUser == "" {
			logrus.Debug("channel_id_for_system_user isn't set")
			return nil
		}
		// treat the user as a system account
		// send a notification to slack
		msg, err := ctrl.RenderTemplate(ctrl.MessageTemplateForSystemUser, map[string]interface{}{
			"UserName":     param.UserName,
			"AWSAccountID": ctrl.Config.AWSAccountID,
		})
		if err != nil {
			return err
		}
		if _, _, _, err := ctrl.SlackBot.SendMessageContext(ctx, ctrl.Config.Slack.ChannelIDForSystemUser, slack.MsgOptionText(msg, false)); err != nil {
			return fmt.Errorf("send a notification that a system user has been created to Slack channel (channel id: %s): %w", ctrl.Config.Slack.ChannelIDForSystemUser, err)
		}
		logE.WithFields(logrus.Fields{
			"channel_id": ctrl.Config.Slack.ChannelIDForSystemUser,
		}).Info("send a notification that a system user has been created to Slack channel")
		return nil
	}
	logE = logE.WithFields(logrus.Fields{
		"slack_id": user.ID,
	})
	logE.Info("get a Slack User")
	// create an initial password
	passwd, err := password.Generate(ctrl.Config.InitialPasswordLength, 10, 10, false, false)
	if err != nil {
		return fmt.Errorf("generate an initial password: %w", err)
	}
	logE.Info("generate an initial password")
	// create a login profile
	iamSvc := iam.New(sess, aws.NewConfig().WithRegion(ctrl.Config.Region))
	if _, err := iamSvc.CreateLoginProfileWithContext(ctx, &iam.CreateLoginProfileInput{
		Password:              aws.String(passwd),
		PasswordResetRequired: aws.Bool(true),
		UserName:              aws.String(param.UserName),
	}); err != nil {
		return fmt.Errorf("create a login profile: %w", err)
	}
	logE.Info("create a login profile")
	// create an access key
	// if _, err := iamSvc.CreateAccessKeyWithContext(ctx, &iam.CreateAccessKeyInput{}); err != nil {
	// 	return err
	// }
	// create a message
	msg, err := ctrl.RenderTemplate(ctrl.MessageTemplate, map[string]interface{}{
		"UserName":     param.UserName,
		"Password":     passwd,
		"AWSAccountID": ctrl.Config.AWSAccountID,
	})
	if err != nil {
		return fmt.Errorf("render a message: %w", err)
	}
	logE.Info("render a message")
	// send a message
	if _, _, _, err := ctrl.SlackBot.SendMessageContext(ctx, user.ID, slack.MsgOptionText(msg, false)); err != nil {
		return fmt.Errorf("send Slack DM to a created user(Slack User ID: %s): %w", user.ID, err)
	}
	logE.Info("send a message")
	return nil
}
