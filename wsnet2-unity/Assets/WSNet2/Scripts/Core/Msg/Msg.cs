namespace WSNet2.Core
{
    public enum MsgType
    {
        regularMsgType = 30,

        Ping = 1,

        Leave = regularMsgType,
        RoomProp,
        ClientProp,
        Target,
        ToMaster,
        Broadcast,
        Kick,
    }
}
